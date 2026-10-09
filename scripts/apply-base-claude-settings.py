#!/usr/bin/env python3
"""Apply public-safe base preference keys into ~/.claude/settings.json.

The fleet-wide keys every host gets whether or not the private overlay is
present. Auto-memory is off because point-in-time memory drifts, and Claude in
Chrome is denied because browser computer-use should be a session opt-in.
Additive and key-scoped: it sets only the keys it owns and preserves the rest
verbatim, so the harness, ward and the bridge merge can share the file.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
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

# Fleet-wide permission denies. See docs/native-claude-credentials.md.
BASE_DENIED_PERMISSIONS = [
    "Edit(**/.claude/projects/**/memory/**)",
]

# The one exception to append-only: dropping a rule from the list above leaves
# it on every converged host. See docs/native-claude-credentials.md.
RETIRED_DENIED_PERMISSIONS = [
    # Live-infrastructure CLIs, retired 2026-09-14. Reasoning in the doc page.
    "Bash(gcloud *)",
    "Bash(kubectl *)",
    "Bash(helm *)",
    "Bash(terraform *)",
    "Bash(gsutil *)",
    "Bash(mongosh *)",
    "Bash(mongo *)",
    # Edit(path) rules cover every file-editing tool. Write(path) matches nothing.
    "Write(**/.claude/projects/**/memory/**)",
    # The GitHub pull-request guard moved to umbra: the guarded binary refuses
    # and states why, so the harness needs no opinion about `gh`.
    "Bash(gh pr create:*)",
    "Bash(gh pr merge:*)",
    "Bash(gh pr edit:*)",
    "Bash(gh pr close:*)",
    "Bash(gh pr reopen:*)",
    "Bash(gh pr ready:*)",
    "Bash(gh pr review:*)",
    "Bash(gh pr comment:*)",
]

# Prefix matches, so these bind only when the host is the first argument.
# A flagged invocation misses them: docs/native-claude-credentials.md.
BASE_ALLOWED_PERMISSIONS: list[str] = [
    "Bash(ssh coilysiren@ser8:*)",
    "Bash(ssh firem@kai-tower-3026:*)",
    # Exact matches, one per area with a `describe` verb. A `*` in the area slot
    # would match spaces too, covering any aosguard line that ends in `describe`.
    "Bash(aosguard ops forgejo describe)",
    "Bash(aosguard ops forgejo-admin describe)",
    # `*` after the program matches the bare command too, so no separate bare
    # or `:*` form. The per-area rules come from `aosguard ops --help` at run time.
    "Bash(aosguard *)",
    "Bash(aosguard ops *)",
    "Bash(aosguard update *)",
]

# Prose rules for the auto mode classifier, merged beside "$defaults". An entry
# is matched by its first 40 characters, so a changed wording replaces in place.
AUTO_MODE_KEY_LEN = 40
BASE_AUTO_MODE_ALLOW: list[str] = [
    "aosguard at any depth is first-party: `aosguard`, `aosguard ops`, each "
    "`aosguard ops <area>` and `aosguard update` are Kai's own guarded operator "
    "CLI, and its specs and grants are the boundary, so the classifier need not "
    "second-guess a verb that aosguard itself grants.",
    "Read-only commands are never denied: git status, git log, git diff, git "
    "show, git rev-parse, ls, cat, head, sed -n, grep, rg, find without -exec "
    "or -delete, wc, date, and taking a screenshot. A denial of one of these is "
    "a classifier bug, not an approval wall. Reading credential or key material "
    "stays with the classifier: ~/.aws, ~/.ssh, .env*, *.pem, *.key, kubeconfig, "
    "and ssm-get or --with-decryption output.",
    "Any `just <verb>` invocation, including verbs that build, test, format, "
    "generate, release, converge or deploy, and including ones whose recipes "
    "shell into other tools. `just` is the sole dev-command entrypoint across "
    "this estate by standing policy, and every verb is declared in a tracked "
    "justfile that is reviewed code in Kai's own repository rather than shell an "
    "agent composed on the spot. Do not deny a `just` verb merely because its "
    "name, or a tool inside its recipe, would be judged differently when typed "
    "directly.",
]


def parse_ops_areas(help_text: str) -> list[str]:
    """Area names from the COMMANDS block of `aosguard ops --help`."""
    areas: list[str] = []
    in_commands = False
    for line in help_text.splitlines():
        if line.startswith("COMMANDS:"):
            in_commands = True
        elif in_commands and not line.strip():
            break
        elif in_commands:
            match = re.match(r"^\s+([a-z][a-z0-9-]*)\s", line)
            if match:
                areas.append(match.group(1))
    return areas


def aosguard_ops_areas() -> list[str]:
    """The areas this host's aosguard grants, empty when it is not installed."""
    exe = shutil.which("aosguard")
    if not exe:
        return []
    try:
        run = subprocess.run(
            [exe, "ops", "--help"], capture_output=True, text=True, timeout=20, check=False
        )
    except (OSError, subprocess.SubprocessError):
        return []
    return parse_ops_areas(run.stdout)
RETIRED_ALLOWED_PERMISSIONS = [
    # The harness refuses a bare wildcard in allow and warns at every session
    # start, so this one was inert from the day it landed (agentic-os#1165).
    "*",
]


def load_settings(path: Path) -> dict:
    if not path.exists():
        return {}
    return json.loads(path.read_text())


def merge_auto_mode_allow(settings: dict, areas: list[str]) -> bool:
    """Merge the owned prose into autoMode.allow, keeping every other entry."""
    auto_mode = settings.get("autoMode")
    if not isinstance(auto_mode, dict):
        auto_mode = {}
    allow = auto_mode.get("allow")
    if not isinstance(allow, list):
        allow = ["$defaults"]
    wanted = list(BASE_AUTO_MODE_ALLOW)
    if areas:
        names = ", ".join(f"`{area}`" for area in areas)
        wanted.insert(1, f"Each `aosguard ops` area is first-party and named here: {names}.")
    changed = False
    for text in wanted:
        key = text[:AUTO_MODE_KEY_LEN]
        index = next(
            (i for i, entry in enumerate(allow) if isinstance(entry, str) and entry.startswith(key)),
            None,
        )
        if index is None:
            allow.append(text)
            changed = True
        elif allow[index] != text:
            allow[index] = text
            changed = True
    auto_mode["allow"] = allow
    settings["autoMode"] = auto_mode
    return changed


def merge_base_settings(settings: dict, areas: list[str] | None = None) -> list[str]:
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
    area_rules = [f"Bash(aosguard ops {area} *)" for area in areas or []]
    for rule in BASE_ALLOWED_PERMISSIONS + area_rules:
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
    if merge_auto_mode_allow(settings, areas or []):
        changed.append("autoMode.allow")
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
    changed = merge_base_settings(settings, aosguard_ops_areas())

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
