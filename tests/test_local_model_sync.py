from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from agentic_os import local_model_sync


def _write_sources(root: Path, *, goose: str = "ministral-3:8b") -> None:
    info = root / ".agents/skills/leaderboard-agent-model-pairs/info"
    info.mkdir(parents=True)
    pairings = {
        "entries": [
            {"agent": "goose", "model": goose, "server": "model-host"},
            {"agent": "openhands", "model": "gpt-oss:120b", "server": "model-host"},
        ]
    }
    inventory = {
        "entries": [
            {"model": "gpt-oss:120b", "server": "model-host", "keep": True},
            {"model": goose, "server": "model-host", "keep": True},
        ]
    }
    (info / "94-pairings.yaml").write_text(yaml.safe_dump(pairings), encoding="utf-8")
    (info / "90-inventory.yaml").write_text(yaml.safe_dump(inventory), encoding="utf-8")


def _bundle(opencode: str = "old-code", goose: str = "old-goose") -> str:
    return f"""agents {{
    schema-version 2
    agent goose {{
        model {goose}
        endpoint \"http://localhost:11434/v1\"
    }}
    agent opencode {{
        model {opencode}
        endpoint \"http://host.docker.internal:8082/v1\"
    }}
}}
"""


def test_load_selections_requires_provisioned_models(tmp_path: Path) -> None:
    _write_sources(tmp_path)
    inventory = tmp_path / local_model_sync.INVENTORY_PATH
    inventory.write_text("entries: []\n", encoding="utf-8")

    with pytest.raises(local_model_sync.SyncError, match="is not uniquely provisioned"):
        local_model_sync.load_selections(tmp_path)


def test_render_bundle_updates_only_selected_model_lines() -> None:
    selected = {
        "goose": local_model_sync.Selection("ministral-3:8b", "model-host"),
    }

    rendered = local_model_sync.render_bundle(_bundle(), selected)

    assert local_model_sync.bundle_models(rendered) == {
        "goose": "ministral-3:8b",
    }
    assert "model old-code" in rendered
    assert "http://localhost:11434/v1" in rendered
    assert "http://host.docker.internal:8082/v1" in rendered


def test_check_reports_drift_without_writing(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    _write_sources(tmp_path)
    bundle = tmp_path / "agents.kdl"
    original = _bundle()
    bundle.write_text(original, encoding="utf-8")

    assert local_model_sync.run(tmp_path, bundle, check=True) == 1
    assert bundle.read_text(encoding="utf-8") == original
    captured = capsys.readouterr()
    assert "drift: goose" in captured.err


def test_sync_writes_then_check_passes(tmp_path: Path) -> None:
    _write_sources(tmp_path)
    bundle = tmp_path / "agents.kdl"
    bundle.write_text(_bundle(), encoding="utf-8")

    assert local_model_sync.run(tmp_path, bundle, check=False) == 0
    assert local_model_sync.run(tmp_path, bundle, check=True) == 0
