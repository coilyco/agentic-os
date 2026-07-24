from __future__ import annotations

import json
import shutil
from collections.abc import Callable
from pathlib import Path

import pytest
import yaml

from agentic_os import role_seat_sync


FIXTURE_ROOT = Path(__file__).parent / "fixtures" / "harness_board"


@pytest.fixture
def role_seat_sources(tmp_path: Path) -> tuple[Path, Path, Path]:
    aosh_root = tmp_path / "aosh"
    shutil.copytree(FIXTURE_ROOT, aosh_root)
    roles_path = aosh_root / "roles.kdl"
    ward_roles_path = tmp_path / "ward-roles.kdl"
    ward_roles_path.write_text(
        (aosh_root / "ward-roles.kdl").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    return aosh_root, roles_path, ward_roles_path


def _rewrite_yaml(path: Path, mutate: Callable[[dict[str, object]], None]) -> None:
    document = yaml.safe_load(path.read_text(encoding="utf-8"))
    assert isinstance(document, dict)
    mutate(document)
    path.write_text(yaml.safe_dump(document, sort_keys=False), encoding="utf-8")


def test_fixture_renders_deterministic_role_seat_identity(
    role_seat_sources: tuple[Path, Path, Path],
) -> None:
    aosh_root, roles_path, ward_roles_path = role_seat_sources
    orientation = role_seat_sync.load_orientation(
        aosh_root,
        roles_path=roles_path,
    )

    rendered = role_seat_sync.merge_ward_roles(
        ward_roles_path.read_text(encoding="utf-8"),
        orientation,
        ward_roles_path,
    )

    assert rendered.count(role_seat_sync.IDENTITY_BEGIN) == orientation.seat_count
    assert rendered.count(role_seat_sync.IDENTITY_END) == orientation.seat_count
    for role in orientation.roles:
        for seat in role.seats:
            assert f"name {json.dumps(seat.name)}" in rendered
            assert f"pronouns {seat.pronouns}" in rendered
    assert 'guardfile "guardfile.example.kdl"' in rendered
    assert "model claude-example" in rendered
    assert "reasoning-effort xhigh" in rendered
    assert '// name "old engineer"' not in rendered
    assert 'name "old director"' not in rendered


def test_sync_writes_then_check_passes(
    role_seat_sources: tuple[Path, Path, Path],
) -> None:
    aosh_root, roles_path, ward_roles_path = role_seat_sources

    assert (
        role_seat_sync.run(
            aosh_root,
            ward_roles_path,
            roles_path=roles_path,
            check=False,
        )
        == 0
    )
    projected = ward_roles_path.read_text(encoding="utf-8")
    assert role_seat_sync.IDENTITY_BEGIN in projected
    assert (
        role_seat_sync.run(
            aosh_root,
            ward_roles_path,
            roles_path=roles_path,
            check=True,
        )
        == 0
    )


def test_check_reports_drift_without_writing(
    role_seat_sources: tuple[Path, Path, Path],
    capsys: pytest.CaptureFixture[str],
) -> None:
    aosh_root, roles_path, ward_roles_path = role_seat_sources
    original = ward_roles_path.read_text(encoding="utf-8")

    assert (
        role_seat_sync.run(
            aosh_root,
            ward_roles_path,
            roles_path=roles_path,
            check=True,
        )
        == 1
    )
    assert ward_roles_path.read_text(encoding="utf-8") == original
    assert f"drift: {ward_roles_path}" in capsys.readouterr().err


def test_present_incomplete_orientation_fails_closed(
    role_seat_sources: tuple[Path, Path, Path],
) -> None:
    aosh_root, roles_path, _ = role_seat_sources
    orientation_path = aosh_root / role_seat_sync.ORIENTATION_PATH

    def mutate(document: dict[str, object]) -> None:
        roles = document["roles"]
        assert isinstance(roles, dict)
        del roles["ops"]

    _rewrite_yaml(orientation_path, mutate)

    with pytest.raises(role_seat_sync.RoleSeatSyncError, match="missing="):
        role_seat_sync.load_orientation(aosh_root, roles_path=roles_path)


def test_duplicate_seat_harness_fails_closed(
    role_seat_sources: tuple[Path, Path, Path],
) -> None:
    aosh_root, roles_path, _ = role_seat_sources
    orientation_path = aosh_root / role_seat_sync.ORIENTATION_PATH

    def mutate(document: dict[str, object]) -> None:
        roles = document["roles"]
        assert isinstance(roles, dict)
        engineer = roles["engineer"]
        assert isinstance(engineer, dict)
        seats = engineer["seats"]
        assert isinstance(seats, list)
        seats.append(dict(seats[0]))

    _rewrite_yaml(orientation_path, mutate)

    with pytest.raises(role_seat_sync.RoleSeatSyncError, match="repeats"):
        role_seat_sync.load_orientation(aosh_root, roles_path=roles_path)


def test_duplicate_role_personality_fails_closed(
    role_seat_sources: tuple[Path, Path, Path],
) -> None:
    aosh_root, roles_path, _ = role_seat_sources
    orientation_path = aosh_root / role_seat_sync.ORIENTATION_PATH

    def mutate(document: dict[str, object]) -> None:
        roles = document["roles"]
        assert isinstance(roles, dict)
        engineer = roles["engineer"]
        assert isinstance(engineer, dict)
        personalities = engineer["personalities"]
        assert isinstance(personalities, list)
        personalities.append(personalities[0])

    _rewrite_yaml(orientation_path, mutate)

    with pytest.raises(role_seat_sync.RoleSeatSyncError, match="personality"):
        role_seat_sync.load_orientation(aosh_root, roles_path=roles_path)


def test_source_seat_requires_an_existing_ward_agent_block(
    role_seat_sources: tuple[Path, Path, Path],
) -> None:
    aosh_root, roles_path, ward_roles_path = role_seat_sources
    orientation_path = aosh_root / role_seat_sync.ORIENTATION_PATH

    def mutate(document: dict[str, object]) -> None:
        roles = document["roles"]
        assert isinstance(roles, dict)
        qa = roles["qa"]
        assert isinstance(qa, dict)
        qa["seats"] = [
            {"harness": "goose", "name": "local tester", "pronouns": "she"}
        ]

    _rewrite_yaml(orientation_path, mutate)
    orientation = role_seat_sync.load_orientation(
        aosh_root,
        roles_path=roles_path,
    )

    with pytest.raises(role_seat_sync.RoleSeatSyncError, match="lacks configured"):
        role_seat_sync.merge_ward_roles(
            ward_roles_path.read_text(encoding="utf-8"),
            orientation,
            ward_roles_path,
        )


def test_if_present_skips_only_an_absent_checkout(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    missing = tmp_path / "missing-aosh"

    assert (
        role_seat_sync.main(
            [
                "--check",
                "--if-present",
                "--aosh-root",
                str(missing),
            ]
        )
        == 0
    )
    assert "skip: AOSH checkout is absent" in capsys.readouterr().out
