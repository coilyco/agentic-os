"""Tests for the public-safe Claude Code base settings merge."""

from __future__ import annotations

import importlib.util
from pathlib import Path


SCRIPT = Path(__file__).parents[1] / "scripts" / "apply-base-claude-settings.py"
SPEC = importlib.util.spec_from_file_location("apply_base_claude_settings", SCRIPT)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def test_base_settings_disable_memory_and_chrome_without_losing_local_denies() -> None:
    settings = {
        "theme": "dark",
        "deniedMcpServers": [{"serverName": "local-browser"}],
    }

    changed = MODULE.merge_base_settings(settings)

    assert settings["autoMemoryEnabled"] is False
    assert settings["theme"] == "dark"
    assert settings["deniedMcpServers"] == [
        {"serverName": "local-browser"},
        {"serverName": "claude-in-chrome"},
    ]
    assert set(changed) == {"autoMemoryEnabled", "deniedMcpServers"}
    assert MODULE.merge_base_settings(settings) == []
