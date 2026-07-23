from __future__ import annotations

import json
import shutil
from collections.abc import Callable
from pathlib import Path

import pytest
import yaml

from agentic_os import harness_board_sync


FIXTURE_ROOT = Path(__file__).parent / "fixtures" / "harness_board"


@pytest.fixture
def board_sources(tmp_path: Path) -> tuple[Path, Path, Path]:
    aosh_root = tmp_path / "aosh"
    shutil.copytree(FIXTURE_ROOT, aosh_root)
    roles_path = aosh_root / "roles.kdl"
    agent_roles_path = tmp_path / "agent-roles.kdl"
    agent_roles_path.write_text(roles_path.read_text(encoding="utf-8"), encoding="utf-8")
    return aosh_root, roles_path, agent_roles_path


def _rewrite_yaml(path: Path, mutate: Callable[[dict[str, object]], None]) -> None:
    document = yaml.safe_load(path.read_text(encoding="utf-8"))
    assert isinstance(document, dict)
    mutate(document)
    path.write_text(yaml.safe_dump(document, sort_keys=False), encoding="utf-8")


def test_fixture_renders_deterministic_model_opaque_projection(
    board_sources: tuple[Path, Path, Path],
) -> None:
    aosh_root, roles_path, agent_roles_path = board_sources

    board = harness_board_sync.load_board(aosh_root, roles_path=roles_path)
    rendered = harness_board_sync.render_board(board)
    rendered_agent = harness_board_sync.merge_agent_roles(
        agent_roles_path.read_text(encoding="utf-8"),
        board,
        agent_roles_path,
    )

    payload = json.loads(rendered)
    assert payload["roles"] == [
        {
            "role": route.role,
            "intents": [
                {"intent": lane.intent, "harness": lane.harness}
                for lane in route.lanes
            ],
        }
        for route in board.roles
    ]
    for route in board.roles:
        assert rendered_agent.count(f"    role {route.role} {{") == 1
        for lane in route.lanes:
            assert f"        intent {lane.intent} {{" in rendered_agent
            assert f"            harness {lane.harness}" in rendered_agent
    assert "role-harnesses {" not in rendered_agent
    assert not {"model", "server", "fallback", "orchestrator", "rationale"} & set(
        rendered.split('"')
    )
    assert not {"model", "server", "fallback", "orchestrator", "rationale"} & set(
        rendered_agent.replace("{", " ").replace("}", " ").split()
    )


@pytest.mark.parametrize("drift_target", ["json", "agent-kdl"])
def test_check_reports_drift_without_writing(
    board_sources: tuple[Path, Path, Path],
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
    drift_target: str,
) -> None:
    aosh_root, roles_path, agent_roles_path = board_sources
    board = harness_board_sync.load_board(aosh_root, roles_path=roles_path)
    output = tmp_path / "role-harnesses.json"
    output.write_text(harness_board_sync.render_board(board), encoding="utf-8")
    if drift_target == "json":
        output.write_text("{}\n", encoding="utf-8")
        current_agent_roles = agent_roles_path.read_text(encoding="utf-8")
        agent_roles_path.write_text(
            harness_board_sync.merge_agent_roles(
                current_agent_roles,
                board,
                agent_roles_path,
            ),
            encoding="utf-8",
        )
    original_output = output.read_text(encoding="utf-8")
    original_agent_roles = agent_roles_path.read_text(encoding="utf-8")

    assert (
        harness_board_sync.run(
            aosh_root,
            output,
            roles_path=roles_path,
            agent_roles_path=agent_roles_path,
            check=True,
        )
        == 1
    )
    assert output.read_text(encoding="utf-8") == original_output
    assert agent_roles_path.read_text(encoding="utf-8") == original_agent_roles
    stderr = capsys.readouterr().err
    expected_drift = output if drift_target == "json" else agent_roles_path
    assert f"drift: {expected_drift}" in stderr


def test_sync_writes_then_check_passes(
    board_sources: tuple[Path, Path, Path], tmp_path: Path
) -> None:
    aosh_root, roles_path, agent_roles_path = board_sources
    output = tmp_path / "role-harnesses.json"

    assert (
        harness_board_sync.run(
            aosh_root,
            output,
            roles_path=roles_path,
            agent_roles_path=agent_roles_path,
            check=False,
        )
        == 0
    )
    projected_roles = agent_roles_path.read_text(encoding="utf-8")
    assert projected_roles.startswith("roles {\n")
    board = harness_board_sync.load_board(aosh_root, roles_path=roles_path)
    assert (
        projected_roles.count(harness_board_sync.ROLE_ROUTES_BEGIN)
        == len(board.roles)
    )
    assert "role-harnesses {" not in projected_roles
    for route in board.roles:
        assert projected_roles.count(f"    role {route.role} {{") == 1
    first_lane = board.roles[0].lanes[0]
    assert f"intent {first_lane.intent} {{" in projected_roles
    assert f"harness {first_lane.harness}" in projected_roles
    assert (
        harness_board_sync.run(
            aosh_root,
            output,
            roles_path=roles_path,
            agent_roles_path=agent_roles_path,
            check=True,
        )
        == 0
    )


def test_if_present_skips_only_an_absent_checkout(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    missing = tmp_path / "missing-aosh"

    assert (
        harness_board_sync.main(
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


def test_present_incomplete_checkout_fails_closed(
    board_sources: tuple[Path, Path, Path],
    capsys: pytest.CaptureFixture[str],
) -> None:
    aosh_root, roles_path, agent_roles_path = board_sources
    (aosh_root / harness_board_sync.SELECTIONS_PATH).unlink()

    assert (
        harness_board_sync.main(
            [
                "--check",
                "--if-present",
                "--aosh-root",
                str(aosh_root),
                "--roles",
                str(roles_path),
                "--agent-roles",
                str(agent_roles_path),
            ]
        )
        == 2
    )
    assert "harness-board-sync: read" in capsys.readouterr().err


@pytest.mark.parametrize("failure", ["missing-lane", "rationale", "incompatible"])
def test_malformed_selections_fail_closed(
    board_sources: tuple[Path, Path, Path], failure: str
) -> None:
    aosh_root, roles_path, _ = board_sources
    selections_path = aosh_root / harness_board_sync.SELECTIONS_PATH

    def mutate(document: dict[str, object]) -> None:
        selections = document["selections"]
        assert isinstance(selections, dict)
        engineer = selections["engineer"]
        assert isinstance(engineer, dict)
        if failure == "missing-lane":
            del engineer["autonomous-coding"]
        elif failure == "rationale":
            selection = engineer["autonomous-coding"]
            assert isinstance(selection, dict)
            selection["rationale"] = "not part of the contract"
        else:
            engineer["autonomous-coding"] = {"agent": "aider"}

    _rewrite_yaml(selections_path, mutate)

    with pytest.raises(harness_board_sync.BoardSyncError):
        harness_board_sync.load_board(aosh_root, roles_path=roles_path)


def test_unattended_intent_belongs_only_to_engineer(
    board_sources: tuple[Path, Path, Path],
) -> None:
    aosh_root, roles_path, _ = board_sources

    def mutate_roles(document: dict[str, object]) -> None:
        roles = document["roles"]
        assert isinstance(roles, dict)
        advisor = roles["advisor"]
        assert isinstance(advisor, dict)
        advisor["intents"] = ["autonomous-coding"]

    def mutate_selections(document: dict[str, object]) -> None:
        selections = document["selections"]
        assert isinstance(selections, dict)
        selections["advisor"] = {"autonomous-coding": {"agent": "openhands"}}

    _rewrite_yaml(aosh_root / harness_board_sync.ROLES_PATH, mutate_roles)
    _rewrite_yaml(aosh_root / harness_board_sync.SELECTIONS_PATH, mutate_selections)

    with pytest.raises(harness_board_sync.BoardSyncError, match="exclusively"):
        harness_board_sync.load_board(aosh_root, roles_path=roles_path)


def test_merge_agent_roles_rejects_malformed_generated_markers(
    board_sources: tuple[Path, Path, Path],
) -> None:
    aosh_root, roles_path, agent_roles_path = board_sources
    board = harness_board_sync.load_board(aosh_root, roles_path=roles_path)
    current = agent_roles_path.read_text(encoding="utf-8")
    first_role = board.roles[0].role
    malformed = current.replace(
        f"    role {first_role} {{\n",
        f"    role {first_role} {{\n"
        f"        {harness_board_sync.ROLE_ROUTES_BEGIN}\n",
        1,
    )

    with pytest.raises(harness_board_sync.BoardSyncError, match="marker pair"):
        harness_board_sync.merge_agent_roles(
            malformed,
            board,
            agent_roles_path,
        )


def test_merge_agent_roles_migrates_legacy_sibling_registry(
    board_sources: tuple[Path, Path, Path],
) -> None:
    aosh_root, roles_path, agent_roles_path = board_sources
    board = harness_board_sync.load_board(aosh_root, roles_path=roles_path)
    legacy = (
        agent_roles_path.read_text(encoding="utf-8").rstrip()
        + "\n\n"
        + harness_board_sync.LEGACY_KDL_BEGIN
        + "\nrole-harnesses {\n    role obsolete {}\n}\n"
        + harness_board_sync.LEGACY_KDL_END
        + "\n"
    )

    merged = harness_board_sync.merge_agent_roles(legacy, board, agent_roles_path)

    assert "role-harnesses {" not in merged
    assert harness_board_sync.LEGACY_KDL_BEGIN not in merged
    for route in board.roles:
        assert merged.count(f"    role {route.role} {{") == 1
