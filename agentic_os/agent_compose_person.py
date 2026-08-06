"""Load Agent Compose's generated person snapshot."""

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path


PERSON_SNAPSHOT_FORMAT = "agent-compose.person-snapshot.v3"
PERSON_SNAPSHOT_SCHEMA_VERSION = 3
SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")


class AgentComposePersonError(RuntimeError):
    """An Agent Compose person snapshot is invalid."""


@dataclass(frozen=True)
class RolePersonalities:
    role: str
    personalities: tuple[str, ...]


@dataclass(frozen=True)
class PersonSnapshot:
    roles: tuple[RolePersonalities, ...]
    skills: tuple[tuple[str, str], ...]


def personality_skill_id(personality: str) -> str:
    return f"personality-{personality}"


def _mapping(value: object, label: str) -> dict[str, object]:
    if not isinstance(value, dict) or not all(
        isinstance(key, str) for key in value
    ):
        raise AgentComposePersonError(f"{label} must be a string-keyed mapping")
    return value


def _slug(value: object, label: str) -> str:
    if not isinstance(value, str) or not SLUG_RE.fullmatch(value):
        raise AgentComposePersonError(f"{label} must be a lowercase slug")
    return value


def load_person_snapshot(path: Path) -> PersonSnapshot:
    """Read and validate Agent Compose's canonical person snapshot."""
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise AgentComposePersonError(f"read {path}: {exc}") from exc
    if (
        not isinstance(document, dict)
        or document.get("format") != PERSON_SNAPSHOT_FORMAT
        or document.get("schema_version") != PERSON_SNAPSHOT_SCHEMA_VERSION
    ):
        raise AgentComposePersonError(
            f"{path}: expected {PERSON_SNAPSHOT_FORMAT} schema "
            f"{PERSON_SNAPSHOT_SCHEMA_VERSION}"
        )

    role_order = document.get("role_order")
    raw_roles = _mapping(document.get("roles"), f"{path}: roles")
    raw_personalities = _mapping(
        document.get("personalities"), f"{path}: personalities"
    )
    if (
        not isinstance(role_order, list)
        or not role_order
        or not all(isinstance(role, str) for role in role_order)
    ):
        raise AgentComposePersonError(f"{path}: role_order is malformed")
    ordered_roles = tuple(
        _slug(role, f"{path}: role_order entry") for role in role_order
    )
    if (
        len(set(ordered_roles)) != len(ordered_roles)
        or set(ordered_roles) != set(raw_roles)
    ):
        raise AgentComposePersonError(
            f"{path}: role_order does not cover roles exactly"
        )

    roles: list[RolePersonalities] = []
    selected: list[str] = []
    selected_set: set[str] = set()
    for role in ordered_roles:
        raw_role = _mapping(raw_roles[role], f"{path}: role {role}")
        raw_meld = raw_role.get("personalities")
        if (
            not isinstance(raw_meld, list)
            or not 2 <= len(raw_meld) <= 4
            or not all(isinstance(value, str) for value in raw_meld)
        ):
            raise AgentComposePersonError(
                f"{path}: role {role} personalities are malformed"
            )
        meld = tuple(
            _slug(value, f"{path}: role {role} personality")
            for value in raw_meld
        )
        if len(set(meld)) != len(meld):
            raise AgentComposePersonError(
                f"{path}: role {role} repeats a personality"
            )
        roles.append(RolePersonalities(role=role, personalities=meld))
        for personality in meld:
            if personality not in selected_set:
                selected.append(personality)
                selected_set.add(personality)

    personality_slugs = {
        _slug(personality, f"{path}: personality key")
        for personality in raw_personalities
    }
    if personality_slugs != selected_set:
        raise AgentComposePersonError(
            f"{path}: personality catalog does not match role selections"
        )
    skills: list[tuple[str, str]] = []
    for personality in selected:
        raw_personality = _mapping(
            raw_personalities[personality],
            f"{path}: personality {personality}",
        )
        skill = raw_personality.get("skill")
        if skill != personality_skill_id(personality):
            raise AgentComposePersonError(
                f"{path}: personality {personality} has invalid skill binding"
            )
        skills.append((personality, skill))
    return PersonSnapshot(roles=tuple(roles), skills=tuple(skills))
