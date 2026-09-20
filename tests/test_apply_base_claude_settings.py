"""Tests for the public-safe Claude Code base settings merge."""

from __future__ import annotations

import importlib.util
import json
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
    assert settings["tui"] == "fullscreen"
    assert settings["theme"] == "dark"
    assert settings["deniedMcpServers"] == [
        {"serverName": "local-browser"},
        {"serverName": "claude-in-chrome"},
    ]
    assert set(changed) == {
        "autoMemoryEnabled",
        "tui",
        "deniedMcpServers",
        "permissions.deny",
        "permissions.allow",
    }
    assert MODULE.merge_base_settings(settings) == []


def test_the_retired_github_rules_come_back_off_a_converged_host() -> None:
    """Append-only is the trap: a rule dropped from BASE_ alone stays forever."""
    settings = {"permissions": {"deny": list(MODULE.RETIRED_DENIED_PERMISSIONS)}}

    MODULE.merge_base_settings(settings)

    assert not [r for r in settings["permissions"]["deny"] if "gh pr " in r]
    assert not [r for r in MODULE.BASE_DENIED_PERMISSIONS if "gh " in r]


def test_permission_rules_append_without_touching_sibling_permission_keys() -> None:
    settings = {
        "permissions": {
            "allow": ["Bash(coily:*)"],
            "deny": ["Bash(rm -rf /*)"],
            "defaultMode": "auto",
        },
    }

    MODULE.merge_base_settings(settings)

    permissions = settings["permissions"]
    assert permissions["allow"][0] == "Bash(coily:*)"
    assert permissions["allow"][1:] == MODULE.BASE_ALLOWED_PERMISSIONS
    assert permissions["defaultMode"] == "auto"
    assert permissions["deny"][0] == "Bash(rm -rf /*)"
    assert permissions["deny"][1:] == MODULE.BASE_DENIED_PERMISSIONS
    assert "Edit(**/.claude/projects/**/memory/**)" in permissions["deny"]


def test_a_retired_deny_is_removed_from_a_converged_host() -> None:
    """Dropping a rule from the base list only stops re-adding it. Retiring is
    what clears it from every host that already has it."""
    settings = {"permissions": {"deny": ["Bash(kubectl *)", "Bash(rm -rf /*)"]}}

    changed = MODULE.merge_base_settings(settings)

    assert "Bash(kubectl *)" not in settings["permissions"]["deny"]
    assert "Bash(rm -rf /*)" in settings["permissions"]["deny"]
    assert "permissions.deny" in changed


def test_permission_rules_are_created_when_the_key_is_absent() -> None:
    settings: dict = {}

    changed = MODULE.merge_base_settings(settings)

    assert settings["permissions"]["deny"] == MODULE.BASE_DENIED_PERMISSIONS
    assert settings["permissions"]["allow"] == MODULE.BASE_ALLOWED_PERMISSIONS
    assert "permissions.deny" in changed
    assert "permissions.allow" in changed
    assert MODULE.merge_base_settings(settings) == []


def test_retired_permission_rules_are_pruned_from_an_already_converged_host() -> None:
    settings = {
        "permissions": {
            "deny": [
                "Write(**/.claude/projects/**/memory/**)",
                "Edit(**/.claude/projects/**/memory/**)",
                "Bash(rm -rf /*)",
            ],
        },
    }

    changed = MODULE.merge_base_settings(settings)

    deny = settings["permissions"]["deny"]
    assert "Write(**/.claude/projects/**/memory/**)" not in deny
    assert "Edit(**/.claude/projects/**/memory/**)" in deny
    assert "Bash(rm -rf /*)" in deny
    assert "permissions.deny" in changed
    assert MODULE.merge_base_settings(settings) == []


def test_retired_allow_rules_are_pruned_from_an_already_converged_host() -> None:
    settings = {
        "permissions": {
            "allow": ["Bash(coily:*)", "*", "Agent"],
        },
    }

    changed = MODULE.merge_base_settings(settings)

    assert settings["permissions"]["allow"] == [
        "Bash(coily:*)",
        "Agent",
    ] + MODULE.BASE_ALLOWED_PERMISSIONS
    assert "permissions.allow" in changed
    assert MODULE.merge_base_settings(settings) == []


def test_no_retired_rule_is_also_a_live_rule() -> None:
    assert not set(MODULE.RETIRED_DENIED_PERMISSIONS) & set(MODULE.BASE_DENIED_PERMISSIONS)
    assert not set(MODULE.RETIRED_ALLOWED_PERMISSIONS) & set(MODULE.BASE_ALLOWED_PERMISSIONS)


def test_write_follows_a_symlink_instead_of_replacing_it(tmp_path) -> None:
    host = tmp_path / "host"
    host.mkdir()
    host_settings = host / "settings.json"
    host_settings.write_text('{"theme": "dark"}\n')
    session = tmp_path / "session"
    session.mkdir()
    link = session / "settings.json"
    link.symlink_to(host_settings)

    target = MODULE.write_settings(link, {"theme": "light"})

    assert link.is_symlink()
    assert target == host_settings.resolve()
    assert json.loads(host_settings.read_text()) == {"theme": "light"}


def test_write_creates_a_plain_file_when_the_path_is_not_a_link(tmp_path) -> None:
    path = tmp_path / "nested" / "settings.json"

    target = MODULE.write_settings(path, {"theme": "dark"})

    assert target == path
    assert not path.is_symlink()
    assert json.loads(path.read_text()) == {"theme": "dark"}
