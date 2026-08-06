from __future__ import annotations

import json
from pathlib import Path

import pytest

from agentic_os import agent_compose_person


def _person_snapshot() -> dict[str, object]:
    return {
        "format": agent_compose_person.PERSON_SNAPSHOT_FORMAT,
        "schema_version": agent_compose_person.PERSON_SNAPSHOT_SCHEMA_VERSION,
        "source": "person:fixture",
        "person": "fixture",
        "role_order": ["builder", "guide"],
        "roles": {
            "builder": {
                "personalities": [
                    "curious",
                    "grounded",
                    "meticulous",
                    "tenacious",
                ],
                "supported_model_classes": ["frontier"],
            },
            "guide": {
                "personalities": ["playful", "diplomatic"],
            },
        },
        "personalities": {
            personality: {
                "skill": agent_compose_person.personality_skill_id(personality)
            }
            for personality in (
                "curious",
                "grounded",
                "meticulous",
                "tenacious",
                "playful",
                "diplomatic",
            )
        },
    }


@pytest.fixture
def person_snapshot(tmp_path: Path) -> Path:
    path = tmp_path / "person.json"
    path.write_text(
        json.dumps(_person_snapshot(), indent=2) + "\n",
        encoding="utf-8",
    )
    return path


def test_load_preserves_ordered_role_personalities(person_snapshot: Path) -> None:
    snapshot = agent_compose_person.load_person_snapshot(person_snapshot)

    assert {
        role.role: role.personalities for role in snapshot.roles
    } == {
        "builder": ("curious", "grounded", "meticulous", "tenacious"),
        "guide": ("playful", "diplomatic"),
    }
    assert snapshot.skills[0] == ("curious", "personality-curious")


def test_rejects_invalid_personality_binding(person_snapshot: Path) -> None:
    document = json.loads(person_snapshot.read_text(encoding="utf-8"))
    document["personalities"]["playful"]["skill"] = "personality-wrong"
    person_snapshot.write_text(json.dumps(document), encoding="utf-8")

    with pytest.raises(
        agent_compose_person.AgentComposePersonError,
        match="invalid skill binding",
    ):
        agent_compose_person.load_person_snapshot(person_snapshot)


def test_rejects_role_order_that_omits_a_role(person_snapshot: Path) -> None:
    document = json.loads(person_snapshot.read_text(encoding="utf-8"))
    document["role_order"].pop()
    person_snapshot.write_text(json.dumps(document), encoding="utf-8")

    with pytest.raises(
        agent_compose_person.AgentComposePersonError,
        match="role_order does not cover roles exactly",
    ):
        agent_compose_person.load_person_snapshot(person_snapshot)
