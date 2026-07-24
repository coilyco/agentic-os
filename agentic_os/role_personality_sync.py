"""Project AOSH role personalities into an AOS provider-alignment artifact."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from agentic_os.generators.generate_agent_compose import _split_frontmatter
from agentic_os.role_seat_sync import (
    DEFAULT_AOSH_ROOT,
    DEFAULT_ROLES,
    ORIENTATION_PATH,
    RoleOrientation,
    RoleSeatSyncError,
    SLUG_RE,
    load_orientation,
)


REPO_ROOT = Path(__file__).resolve().parents[1]
PROJECTION_PATH = Path("aos") / "role-personalities.json"
DEFAULT_OUTPUT = REPO_ROOT / PROJECTION_PATH
DEFAULT_SKILLS_ROOT = REPO_ROOT / ".agents" / "skills"
FORMAT = "agentic-os.role-personality-board.v1"
INVARIANT_PATH = Path("personality-shared") / "INVARIANT.md"


class RolePersonalitySyncError(RuntimeError):
    """An AOSH personality projection or AOS provider invariant is invalid."""


def personality_skill_id(personality: str) -> str:
    return f"personality-{personality}"


def _ordered_personalities(orientation: RoleOrientation) -> tuple[str, ...]:
    ordered: list[str] = []
    seen: set[str] = set()
    for role in orientation.roles:
        for personality in role.personalities:
            if personality not in seen:
                ordered.append(personality)
                seen.add(personality)
    return tuple(ordered)


def validate_catalog(
    orientation: RoleOrientation,
    skills_root: Path,
) -> None:
    invariant = skills_root / INVARIANT_PATH
    try:
        invariant_text = invariant.read_text(encoding="utf-8")
    except OSError as exc:
        raise RolePersonalitySyncError(f"read {invariant}: {exc}") from exc
    if not invariant_text.strip():
        raise RolePersonalitySyncError(f"{invariant}: invariant must be non-empty")

    for personality in _ordered_personalities(orientation):
        skill_id = personality_skill_id(personality)
        skill_root = skills_root / skill_id
        entrypoint = skill_root / "SKILL.md"
        try:
            entrypoint_text = entrypoint.read_text(encoding="utf-8")
        except OSError as exc:
            raise RolePersonalitySyncError(
                f"AOSH personality {personality} lacks AOS body {skill_root}: {exc}"
            ) from exc
        metadata, body = _split_frontmatter(entrypoint_text)
        if metadata.get("name") != skill_id:
            raise RolePersonalitySyncError(
                f"{entrypoint}: name must be {skill_id}"
            )
        description = metadata.get("description")
        if not isinstance(description, str) or not description.strip():
            raise RolePersonalitySyncError(
                f"{entrypoint}: description must be non-empty text"
            )
        if not body.strip():
            raise RolePersonalitySyncError(
                f"{skill_root}: SKILL.md body must be non-empty"
            )


def render_projection(orientation: RoleOrientation) -> str:
    personalities = _ordered_personalities(orientation)
    payload = {
        "format": FORMAT,
        "role_count": len(orientation.roles),
        "personality_count": len(personalities),
        "roles": [
            {
                "role": role.role,
                "personalities": list(role.personalities),
            }
            for role in orientation.roles
        ],
        "skills": [
            {
                "personality": personality,
                "skill": personality_skill_id(personality),
            }
            for personality in personalities
        ],
    }
    return json.dumps(payload, indent=2) + "\n"


def load_projection(path: Path) -> dict[str, tuple[str, ...]]:
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RolePersonalitySyncError(f"read {path}: {exc}") from exc
    if not isinstance(document, dict) or document.get("format") != FORMAT:
        raise RolePersonalitySyncError(f"{path}: unsupported projection format")
    roles = document.get("roles")
    skills = document.get("skills")
    if not isinstance(roles, list) or not isinstance(skills, list):
        raise RolePersonalitySyncError(
            f"{path}: roles and skills must be lists"
        )

    role_map: dict[str, tuple[str, ...]] = {}
    selected: set[str] = set()
    for index, raw_role in enumerate(roles):
        if not isinstance(raw_role, dict) or set(raw_role) != {
            "role",
            "personalities",
        }:
            raise RolePersonalitySyncError(f"{path}: roles[{index}] is malformed")
        role = raw_role["role"]
        personalities = raw_role["personalities"]
        if (
            not isinstance(role, str)
            or not SLUG_RE.fullmatch(role)
            or role in role_map
            or not isinstance(personalities, list)
            or not personalities
            or not all(
                isinstance(value, str) and SLUG_RE.fullmatch(value)
                for value in personalities
            )
            or len(set(personalities)) != len(personalities)
        ):
            raise RolePersonalitySyncError(f"{path}: roles[{index}] is malformed")
        role_map[role] = tuple(personalities)
        selected.update(personalities)

    bindings: dict[str, str] = {}
    for index, raw_binding in enumerate(skills):
        if not isinstance(raw_binding, dict) or set(raw_binding) != {
            "personality",
            "skill",
        }:
            raise RolePersonalitySyncError(f"{path}: skills[{index}] is malformed")
        personality = raw_binding["personality"]
        skill = raw_binding["skill"]
        if (
            not isinstance(personality, str)
            or not SLUG_RE.fullmatch(personality)
            or personality in bindings
            or not isinstance(skill, str)
            or skill != personality_skill_id(personality)
        ):
            raise RolePersonalitySyncError(f"{path}: skills[{index}] is malformed")
        bindings[personality] = skill

    if set(bindings) != selected:
        raise RolePersonalitySyncError(
            f"{path}: personality bindings do not match role selections"
        )
    if document.get("role_count") != len(role_map):
        raise RolePersonalitySyncError(f"{path}: role_count is inconsistent")
    if document.get("personality_count") != len(bindings):
        raise RolePersonalitySyncError(
            f"{path}: personality_count is inconsistent"
        )
    return role_map


def run(
    aosh_root: Path,
    output: Path,
    *,
    roles_path: Path = DEFAULT_ROLES,
    skills_root: Path = DEFAULT_SKILLS_ROOT,
    check: bool,
) -> int:
    try:
        orientation = load_orientation(aosh_root, roles_path=roles_path)
    except RoleSeatSyncError as exc:
        raise RolePersonalitySyncError(str(exc)) from exc
    validate_catalog(orientation, skills_root)
    expected = render_projection(orientation)
    try:
        current = output.read_text(encoding="utf-8")
    except FileNotFoundError:
        current = ""
    except OSError as exc:
        raise RolePersonalitySyncError(f"read {output}: {exc}") from exc
    if current == expected:
        print(
            "ok: AOS role personalities match AOSH and the personality catalog "
            f"({len(orientation.roles)} roles, "
            f"{len(_ordered_personalities(orientation))} personalities)"
        )
        return 0
    if check:
        print(
            f"drift: {output} does not match {aosh_root / ORIENTATION_PATH}",
            file=sys.stderr,
        )
        print(
            "run `ward exec sync-role-personalities` from the AOS checkout",
            file=sys.stderr,
        )
        return 1

    try:
        output.write_text(expected, encoding="utf-8")
    except OSError as exc:
        raise RolePersonalitySyncError(f"write {output}: {exc}") from exc
    print(f"updated {output} from {aosh_root / ORIENTATION_PATH}")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check",
        action="store_true",
        help="report drift without changing the projection",
    )
    parser.add_argument(
        "--if-present",
        action="store_true",
        help="skip when the default sibling AOSH checkout is absent",
    )
    parser.add_argument("--aosh-root", type=Path, default=DEFAULT_AOSH_ROOT)
    parser.add_argument("--roles", type=Path, default=DEFAULT_ROLES)
    parser.add_argument("--skills-root", type=Path, default=DEFAULT_SKILLS_ROOT)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args(argv)

    if args.if_present and not args.aosh_root.exists():
        print(f"skip: AOSH checkout is absent at {args.aosh_root}")
        return 0
    try:
        return run(
            args.aosh_root,
            args.output,
            roles_path=args.roles,
            skills_root=args.skills_root,
            check=args.check,
        )
    except RolePersonalitySyncError as exc:
        print(f"role-personality-sync: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
