#!/usr/bin/env python3
"""Apply public-safe base preference keys into ~/.claude/settings.json.

Holds the fleet-wide, public-safe Claude Code settings keys that every host
gets regardless of whether the private bridge overlay is present. Auto-memory
is off because point-in-time memory drifts. The TUI is fullscreen because that
is the fleet default, with CLAUDE_CODE_NO_FLICKER=0 as the per-host opt-out. Claude in Chrome is denied because
browser computer-use should be an explicit session opt-in. The permission deny
list keeps live-infrastructure CLIs and the memory directory out of an agent's
raw shell.

Additive and key-scoped: it sets only the keys it owns and preserves every
other key verbatim, so the harness, ward, and the bridge merge can all keep
touching the same file. Idempotent, stdlib only, atomic write. Run by the
claude-hooks ansible role on every host.

Usage:
    scripts/apply-base-claude-settings.py            # write the merged settings
    scripts/apply-base-claude-settings.py --dry-run  # print the result, do not write
"""
from __future__ import annotations

import argparse
import json
import os
import tempfile
from pathlib import Path

HOME = Path.home()
SETTINGS_PATH = HOME / ".claude" / "settings.json"

# Public-safe keys applied on every host. Private/personal keys stay in the
# bridge overlay's own merge, never here.

# effortLevel is deliberately absent: operator-local preference, not a fleet
# guardrail. Reasoning in docs/native-claude-credentials.md.
BASE_SETTINGS: dict = {
    "autoMemoryEnabled": False,
    # The env var CLAUDE_CODE_NO_FLICKER outranks this key, so a host whose
    # terminal cannot take the alternate screen opts out in ~/.shellrc.local.
    "tui": "fullscreen",
}
BASE_DENIED_MCP_SERVERS = [{"serverName": "claude-in-chrome"}]

# Fleet-wide permission denies: live-infrastructure CLIs that belong to a
# guarded verb, plus the memory dir. See docs/native-claude-credentials.md.
BASE_DENIED_PERMISSIONS = [
    "Bash(gcloud *)",
    "Bash(kubectl *)",
    "Bash(helm *)",
    "Bash(terraform *)",
    "Bash(gsutil *)",
    "Bash(mongosh *)",
    "Bash(mongo *)",
    "Edit(**/.claude/projects/**/memory/**)",
]

# The one exception to append-only: dropping a rule from the list above leaves
# it on every converged host. See docs/native-claude-credentials.md.
RETIRED_DENIED_PERMISSIONS = [
    # Edit(path) rules cover every file-editing tool. Write(path) matches nothing.
    "Write(**/.claude/projects/**/memory/**)",
]

# No fleet allow rule. Prompt suppression is defaultMode's job, not an allow
# rule's, and defaultMode is operator-local like effortLevel below.
BASE_ALLOWED_PERMISSIONS: list[str] = []
RETIRED_ALLOWED_PERMISSIONS = [
    # The harness refuses a bare wildcard in allow and warns at every session
    # start, so this one was inert from the day it landed (agentic-os#1165).
    "*",
]


def load_settings(path: Path) -> dict:
    if not path.exists():
        return {}
    return json.loads(path.read_text())


def merge_base_settings(settings: dict) -> list[str]:
    """Set each base key when missing or differing. Returns the keys changed."""
    changed = []
    for key, value in BASE_SETTINGS.items():
        if settings.get(key) != value:
            settings[key] = value
            changed.append(key)
    denied = settings.get("deniedMcpServers")
    if not isinstance(denied, list):
        denied = []
    for entry in BASE_DENIED_MCP_SERVERS:
        if entry not in denied:
            denied.append(entry)
            if "deniedMcpServers" not in changed:
                changed.append("deniedMcpServers")
    settings["deniedMcpServers"] = denied

    # Append-only against deny and allow, so an operator's own rules and the
    # sibling ask/defaultMode keys survive untouched.
    permissions = settings.get("permissions")
    if not isinstance(permissions, dict):
        permissions = {}
    deny = permissions.get("deny")
    if not isinstance(deny, list):
        deny = []
    for rule in BASE_DENIED_PERMISSIONS:
        if rule not in deny:
            deny.append(rule)
            if "permissions.deny" not in changed:
                changed.append("permissions.deny")
    for rule in RETIRED_DENIED_PERMISSIONS:
        while rule in deny:
            deny.remove(rule)
            if "permissions.deny" not in changed:
                changed.append("permissions.deny")
    permissions["deny"] = deny
    allow = permissions.get("allow")
    if not isinstance(allow, list):
        allow = []
    for rule in BASE_ALLOWED_PERMISSIONS:
        if rule not in allow:
            allow.append(rule)
            if "permissions.allow" not in changed:
                changed.append("permissions.allow")
    for rule in RETIRED_ALLOWED_PERMISSIONS:
        while rule in allow:
            allow.remove(rule)
            if "permissions.allow" not in changed:
                changed.append("permissions.allow")
    permissions["allow"] = allow
    settings["permissions"] = permissions
    return changed


def write_settings(path: Path, settings: dict) -> Path:
    """Write the settings, returning the path that actually took the write."""
    # os.replace swaps a symlink itself, cutting a staged shadow home loose
    # from the host file it points at. See docs/native-shadow.md.
    target = Path(os.path.realpath(path))
    target.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=target.parent, suffix=".json")
    with os.fdopen(fd, "w") as handle:
        json.dump(settings, handle, indent=2)
        handle.write("\n")
    os.replace(tmp, target)
    return target


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    settings = load_settings(SETTINGS_PATH)
    changed = merge_base_settings(settings)

    if args.dry_run:
        print(json.dumps(settings, indent=2))
        return 0

    if not changed:
        print(f"base settings unchanged in {os.path.realpath(SETTINGS_PATH)}")
        return 0

    target = write_settings(SETTINGS_PATH, settings)
    print(f"wrote   {target} (base settings: {', '.join(changed)})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
