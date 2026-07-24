"""Project AOSH role-seat identity into AOS Ward agent configuration."""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

from agentic_os.harness_board_sync import BoardSyncError, load_canonical_roles


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_AOSH_ROOT = REPO_ROOT.parents[1] / "coilyco-bridge" / "agentic-os-hardware"
DEFAULT_ROLES = REPO_ROOT / ".agents" / "roles.kdl"
DEFAULT_WARD_ROLES = REPO_ROOT / ".ward" / "roles.kdl"
ORIENTATION_PATH = Path("role-orientation.yaml")
FORMAT = "agent-compose.person-snapshot.v1"
SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")
ROLE_OPEN_RE = re.compile(r"^([ \t]*)role\s+([a-z0-9][a-z0-9-]*)\s+\{$")
ROLE_EMPTY_RE = re.compile(
    r"^([ \t]*)role\s+([a-z0-9][a-z0-9-]*)\s+\{\s*\}$"
)
AGENT_OPEN_RE = re.compile(r"^([ \t]*)agent\s+([a-z0-9][a-z0-9-]*)\s+\{$")
NAME_RE = re.compile(r'^[ \t]*(?://[ \t]*)?name(?:[ \t]+).+$')
PRONOUNS_RE = re.compile(r"^[ \t]*(?://[ \t]*)?pronouns(?:[ \t]+).+$")
MODEL_RE = re.compile(r"^[ \t]*model(?:[ \t]+).+$")
IDENTITY_BEGIN = "// BEGIN generated AOSH role-seat identity"
IDENTITY_END = "// END generated AOSH role-seat identity"


class RoleSeatSyncError(RuntimeError):
    """An AOSH role-seat source or AOS projection invariant is invalid."""


@dataclass(frozen=True)
class SeatIdentity:
    harness: str
    name: str
    pronouns: str


@dataclass(frozen=True)
class RoleIdentity:
    role: str
    personalities: tuple[str, ...]
    seats: tuple[SeatIdentity, ...]


@dataclass(frozen=True)
class RoleOrientation:
    roles: tuple[RoleIdentity, ...]

    @property
    def seat_count(self) -> int:
        return sum(len(role.seats) for role in self.roles)


def _load_yaml(path: Path) -> dict[str, Any]:
    try:
        value = yaml.safe_load(path.read_text(encoding="utf-8"))
    except (OSError, yaml.YAMLError) as exc:
        raise RoleSeatSyncError(f"read {path}: {exc}") from exc
    if not isinstance(value, dict):
        raise RoleSeatSyncError(f"{path}: expected a YAML mapping")
    return value


def _mapping(value: object, label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise RoleSeatSyncError(f"{label} must be a mapping")
    return value


def _check_keys(actual: object, expected: set[str], label: str) -> None:
    if not isinstance(actual, dict):
        raise RoleSeatSyncError(f"{label} must be a mapping")
    if not all(isinstance(key, str) for key in actual):
        raise RoleSeatSyncError(f"{label} keys must be strings")
    actual_keys = set(actual)
    unknown = sorted(actual_keys - expected)
    missing = sorted(expected - actual_keys)
    if unknown or missing:
        raise RoleSeatSyncError(
            f"{label} mismatch, unknown={unknown}, missing={missing}"
        )


def _slug(value: object, label: str) -> str:
    if not isinstance(value, str) or not SLUG_RE.fullmatch(value):
        raise RoleSeatSyncError(f"{label} must be a lowercase hyphenated slug")
    return value


def _text(value: object, label: str) -> str:
    if (
        not isinstance(value, str)
        or not value
        or value != value.strip()
        or "\n" in value
        or "\r" in value
    ):
        raise RoleSeatSyncError(f"{label} must be non-empty single-line text")
    return value


def load_orientation(
    aosh_root: Path, *, roles_path: Path = DEFAULT_ROLES
) -> RoleOrientation:
    orientation_file = aosh_root / ORIENTATION_PATH
    document = _load_yaml(orientation_file)
    if document.get("format") != FORMAT:
        raise RoleSeatSyncError(
            f"{orientation_file}: format must be {FORMAT}"
        )
    roles = _mapping(document.get("roles"), f"{orientation_file}: roles")
    try:
        canonical_roles = load_canonical_roles(roles_path)
    except BoardSyncError as exc:
        raise RoleSeatSyncError(str(exc)) from exc
    _check_keys(roles, set(canonical_roles), f"{orientation_file}: roles")

    identities: list[RoleIdentity] = []
    for role in canonical_roles:
        role_spec = _mapping(roles[role], f"{orientation_file}: {role}")
        _check_keys(
            role_spec,
            {"purpose", "personalities", "seats"},
            f"{orientation_file}: {role}",
        )
        raw_personalities = role_spec["personalities"]
        if not isinstance(raw_personalities, list) or not raw_personalities:
            raise RoleSeatSyncError(
                f"{orientation_file}: {role}.personalities must be a non-empty list"
            )
        personalities = tuple(
            _slug(
                personality,
                f"{orientation_file}: {role}.personalities[{index}]",
            )
            for index, personality in enumerate(raw_personalities)
        )
        if len(set(personalities)) != len(personalities):
            raise RoleSeatSyncError(
                f"{orientation_file}: {role} repeats a personality"
            )
        raw_seats = role_spec["seats"]
        if not isinstance(raw_seats, list):
            raise RoleSeatSyncError(
                f"{orientation_file}: {role}.seats must be a list"
            )

        seats: list[SeatIdentity] = []
        seen_harnesses: set[str] = set()
        for index, raw_seat in enumerate(raw_seats):
            label = f"{orientation_file}: {role}.seats[{index}]"
            seat = _mapping(raw_seat, label)
            _check_keys(seat, {"harness", "name", "pronouns"}, label)
            harness = _slug(seat["harness"], f"{label}.harness")
            if harness in seen_harnesses:
                raise RoleSeatSyncError(
                    f"{orientation_file}: {role} repeats seat harness {harness}"
                )
            seen_harnesses.add(harness)
            seats.append(
                SeatIdentity(
                    harness=harness,
                    name=_text(seat["name"], f"{label}.name"),
                    pronouns=_slug(seat["pronouns"], f"{label}.pronouns"),
                )
            )
        identities.append(
            RoleIdentity(
                role=role,
                personalities=personalities,
                seats=tuple(seats),
            )
        )
    return RoleOrientation(roles=tuple(identities))


def _block_close(
    lines: list[str],
    open_index: int,
    indent: str,
    *,
    label: str,
    path: Path,
) -> int:
    try:
        return lines.index(f"{indent}}}", open_index + 1)
    except ValueError as exc:
        raise RoleSeatSyncError(f"{path}: {label} has no closing brace") from exc


def _identity_lines(seat: SeatIdentity, indent: str) -> list[str]:
    return [
        f"{indent}{IDENTITY_BEGIN}",
        f"{indent}name {json.dumps(seat.name, ensure_ascii=False)}",
        f"{indent}pronouns {seat.pronouns}",
        f"{indent}{IDENTITY_END}",
    ]


def _merge_identity_body(
    body: list[str],
    seat: SeatIdentity | None,
    indent: str,
    *,
    label: str,
    path: Path,
) -> list[str]:
    begin_indexes = [
        index for index, line in enumerate(body) if line.strip() == IDENTITY_BEGIN
    ]
    end_indexes = [
        index for index, line in enumerate(body) if line.strip() == IDENTITY_END
    ]
    if len(begin_indexes) != len(end_indexes) or len(begin_indexes) > 1:
        raise RoleSeatSyncError(
            f"{path}: {label} has a malformed generated identity marker pair"
        )

    if begin_indexes:
        begin_index = begin_indexes[0]
        end_index = end_indexes[0]
        if end_index < begin_index:
            raise RoleSeatSyncError(
                f"{path}: {label} generated identity markers are out of order"
            )
        outside = body[:begin_index] + body[end_index + 1 :]
        if any(NAME_RE.fullmatch(line) or PRONOUNS_RE.fullmatch(line) for line in outside):
            raise RoleSeatSyncError(
                f"{path}: {label} has identity fields outside its generated region"
            )
        replacement = _identity_lines(seat, indent) if seat is not None else []
        return body[:begin_index] + replacement + body[end_index + 1 :]

    if seat is None:
        return body

    name_indexes = [
        index for index, line in enumerate(body) if NAME_RE.fullmatch(line)
    ]
    pronoun_indexes = [
        index for index, line in enumerate(body) if PRONOUNS_RE.fullmatch(line)
    ]
    if len(name_indexes) > 1 or len(pronoun_indexes) > 1:
        raise RoleSeatSyncError(
            f"{path}: {label} has duplicate unmarked identity fields"
        )
    identity_indexes = set(name_indexes + pronoun_indexes)
    cleaned = [
        line for index, line in enumerate(body) if index not in identity_indexes
    ]
    model_indexes = [
        index for index, line in enumerate(cleaned) if MODEL_RE.fullmatch(line)
    ]
    if len(model_indexes) > 1:
        raise RoleSeatSyncError(f"{path}: {label} has duplicate model fields")
    insertion_index = model_indexes[0] + 1 if model_indexes else 0
    return (
        cleaned[:insertion_index]
        + _identity_lines(seat, indent)
        + cleaned[insertion_index:]
    )


def merge_ward_roles(
    current: str,
    orientation: RoleOrientation,
    path: Path,
) -> str:
    lines = current.splitlines()
    for role_identity in reversed(orientation.roles):
        role_openings = [
            (index, match.group(1))
            for index, line in enumerate(lines)
            if (match := ROLE_OPEN_RE.fullmatch(line)) is not None
            and match.group(2) == role_identity.role
        ]
        empty_role_indexes = [
            index
            for index, line in enumerate(lines)
            if (match := ROLE_EMPTY_RE.fullmatch(line)) is not None
            and match.group(2) == role_identity.role
        ]
        role_count = len(role_openings) + len(empty_role_indexes)
        if role_count != 1:
            raise RoleSeatSyncError(
                f"{path}: expected one role {role_identity.role}, "
                f"found {role_count}"
            )
        if empty_role_indexes:
            if role_identity.seats:
                raise RoleSeatSyncError(
                    f"{path}: role {role_identity.role} lacks configured agents "
                    f"for AOSH seats "
                    f"{sorted(seat.harness for seat in role_identity.seats)}"
                )
            continue
        role_open_index, role_indent = role_openings[0]
        role_close_index = _block_close(
            lines,
            role_open_index,
            role_indent,
            label=f"role {role_identity.role}",
            path=path,
        )

        agent_blocks: dict[str, tuple[int, int, str]] = {}
        for index in range(role_open_index + 1, role_close_index):
            match = AGENT_OPEN_RE.fullmatch(lines[index])
            if match is None:
                continue
            agent_indent, harness = match.groups()
            if len(agent_indent) <= len(role_indent):
                continue
            if harness in agent_blocks:
                raise RoleSeatSyncError(
                    f"{path}: role {role_identity.role} repeats agent {harness}"
                )
            close_index = _block_close(
                lines,
                index,
                agent_indent,
                label=f"role {role_identity.role} agent {harness}",
                path=path,
            )
            if close_index > role_close_index:
                raise RoleSeatSyncError(
                    f"{path}: role {role_identity.role} agent {harness} "
                    "escapes its role block"
                )
            agent_blocks[harness] = (index, close_index, agent_indent)

        seats = {seat.harness: seat for seat in role_identity.seats}
        missing_agents = sorted(set(seats) - set(agent_blocks))
        if missing_agents:
            raise RoleSeatSyncError(
                f"{path}: role {role_identity.role} lacks configured agents "
                f"for AOSH seats {missing_agents}"
            )

        for harness, (open_index, close_index, agent_indent) in sorted(
            agent_blocks.items(),
            key=lambda item: item[1][0],
            reverse=True,
        ):
            merged_body = _merge_identity_body(
                lines[open_index + 1 : close_index],
                seats.get(harness),
                agent_indent + "    ",
                label=f"role {role_identity.role} agent {harness}",
                path=path,
            )
            lines[open_index + 1 : close_index] = merged_body

    begin_count = sum(line.strip() == IDENTITY_BEGIN for line in lines)
    end_count = sum(line.strip() == IDENTITY_END for line in lines)
    if begin_count != orientation.seat_count or end_count != orientation.seat_count:
        raise RoleSeatSyncError(
            f"{path}: expected {orientation.seat_count} generated identity "
            f"marker pairs, found begin={begin_count}, end={end_count}"
        )
    return "\n".join(lines).rstrip() + "\n"


def run(
    aosh_root: Path,
    ward_roles_path: Path,
    *,
    roles_path: Path = DEFAULT_ROLES,
    check: bool,
) -> int:
    orientation = load_orientation(aosh_root, roles_path=roles_path)
    try:
        current = ward_roles_path.read_text(encoding="utf-8")
    except OSError as exc:
        raise RoleSeatSyncError(f"read {ward_roles_path}: {exc}") from exc
    expected = merge_ward_roles(current, orientation, ward_roles_path)
    if current == expected:
        print(
            "ok: AOS Ward role-agent identities match AOSH "
            f"({len(orientation.roles)} roles, {orientation.seat_count} seats)"
        )
        return 0
    if check:
        print(
            f"drift: {ward_roles_path} does not match "
            f"{aosh_root / ORIENTATION_PATH}",
            file=sys.stderr,
        )
        print("run `ward exec sync-role-seats` from the AOS checkout", file=sys.stderr)
        return 1

    try:
        ward_roles_path.write_text(expected, encoding="utf-8")
    except OSError as exc:
        raise RoleSeatSyncError(
            f"write role-seat agent configuration {ward_roles_path}: {exc}"
        ) from exc
    print(f"updated {ward_roles_path} from {aosh_root / ORIENTATION_PATH}")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check", action="store_true", help="report drift without changing the projection"
    )
    parser.add_argument(
        "--if-present",
        action="store_true",
        help="skip when the default sibling AOSH checkout is absent",
    )
    parser.add_argument("--aosh-root", type=Path, default=DEFAULT_AOSH_ROOT)
    parser.add_argument("--roles", type=Path, default=DEFAULT_ROLES)
    parser.add_argument("--ward-roles", type=Path, default=DEFAULT_WARD_ROLES)
    args = parser.parse_args(argv)

    if args.if_present and not args.aosh_root.exists():
        print(f"skip: AOSH checkout is absent at {args.aosh_root}")
        return 0
    try:
        return run(
            args.aosh_root,
            args.ward_roles,
            roles_path=args.roles,
            check=args.check,
        )
    except RoleSeatSyncError as exc:
        print(f"role-seat-sync: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
