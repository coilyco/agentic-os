"""Behavior tests for host and dev-base status-line composition."""

from __future__ import annotations

import importlib.util
import json
import os
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
COMPOSER = ROOT / "docker" / "dev-base" / "statusline.sh"
COMPOSE_PROVIDER = ROOT / "docker" / "dev-base" / "statusline.d" / "15-agent-compose.sh"
INSTALLER = ROOT / "scripts" / "install-session-name.py"


def _executable(path: Path, content: str) -> None:
    path.write_text(content)
    path.chmod(0o755)


def _load_installer():
    spec = importlib.util.spec_from_file_location("install_session_name", INSTALLER)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_composer_exports_payload_project_directory(tmp_path: Path) -> None:
    providers = tmp_path / "providers"
    providers.mkdir()
    _executable(
        providers / "10-project.sh",
        "#!/usr/bin/env bash\nprintf '%s' \"$AOS_STATUSLINE_PROJECT_DIR\"\n",
    )
    project = tmp_path / "project with spaces"
    project.mkdir()
    env = os.environ | {
        "AOS_STATUSLINE_DIR": str(providers),
        "HOME": str(tmp_path / "home"),
        "XDG_CONFIG_HOME": str(tmp_path / "config"),
    }
    result = subprocess.run(
        [str(COMPOSER)],
        input=json.dumps({"workspace": {"project_dir": str(project)}}),
        text=True,
        capture_output=True,
        check=True,
        env=env,
    )
    assert result.stdout == str(project)


def test_agent_compose_provider_delegates_rendering(tmp_path: Path) -> None:
    binary_dir = tmp_path / "bin"
    binary_dir.mkdir()
    _executable(binary_dir / "acompose", "#!/bin/sh\nprintf '%s' \"$*\"\n")
    project = tmp_path / "project with spaces"
    env = os.environ | {
        "AOS_STATUSLINE_PROJECT_DIR": str(project),
        "PATH": f"{binary_dir}{os.pathsep}{os.environ['PATH']}",
    }
    result = subprocess.run(
        [str(COMPOSE_PROVIDER)], text=True, capture_output=True, check=True, env=env
    )
    assert result.stdout == f"statusline --target {project} --color"


def test_installer_migrates_only_its_legacy_status_line(capsys) -> None:
    installer = _load_installer()
    settings = {
        "statusLine": {
            "type": "command",
            "command": "/opt/agentic-os/agent-name.sh statusline",
            "padding": 2,
        }
    }
    assert installer.merge_statusline(settings)
    assert settings["statusLine"]["command"] == installer.STATUSLINE_CMD
    assert not installer.merge_statusline(settings)

    custom = {"statusLine": {"type": "command", "command": "my-status"}}
    assert not installer.merge_statusline(custom)
    assert custom["statusLine"]["command"] == "my-status"
    assert "not overwriting" in capsys.readouterr().out


def test_installer_repoints_a_retired_sessionstart_hook() -> None:
    # A host converged before agent-name.sh was deleted still has it
    # registered. Skipping it would leave a missing script wired forever.
    installer = _load_installer()
    settings = {
        "hooks": {
            "SessionStart": [
                {
                    "matcher": "startup|resume|clear",
                    "hooks": [
                        {"type": "command", "command": "/old/agent-name.sh sessionstart"}
                    ],
                }
            ]
        }
    }
    assert installer.merge_sessionstart_hook(settings)
    wired = settings["hooks"]["SessionStart"][0]["hooks"][0]["command"]
    assert wired == installer.SESSIONSTART_CMD
    assert "agent-name.sh" not in wired
    assert not installer.merge_sessionstart_hook(settings)


def test_installer_leaves_a_foreign_sessionstart_hook_alone() -> None:
    installer = _load_installer()
    settings = {
        "hooks": {
            "SessionStart": [
                {"matcher": "startup", "hooks": [{"type": "command", "command": "mine"}]}
            ]
        }
    }
    assert installer.merge_sessionstart_hook(settings)
    commands = [
        hook["command"]
        for group in settings["hooks"]["SessionStart"]
        for hook in group["hooks"]
    ]
    assert "mine" in commands
    assert installer.SESSIONSTART_CMD in commands


def test_installer_writes_through_a_symlinked_settings_file(tmp_path: Path) -> None:
    module = _load_installer()
    host = tmp_path / "host"
    host.mkdir()
    host_settings = host / "settings.json"
    host_settings.write_text('{"theme": "dark"}\n')
    session = tmp_path / "session"
    session.mkdir()
    link = session / "settings.json"
    link.symlink_to(host_settings)

    target = module.write_settings(link, {"theme": "light"})

    assert link.is_symlink()
    assert target == host_settings.resolve()
    assert json.loads(host_settings.read_text()) == {"theme": "light"}
