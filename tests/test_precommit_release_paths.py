"""Every hook script must trigger the aos-precommit release that ships it.

aos-precommit-release.yml filters on `paths`. A hook whose script is absent
from that list goes stale silently: fixing the script alone cuts no release,
moves no tag, and consumers pinned to rev=aos-precommit-vX.Y.Z keep running
the old copy with no signal. Sibling of the agentic-os#187 rev/hook-list
invariant, filed as agentic-os#1148.
"""
from __future__ import annotations

from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parent.parent
HOOKS = ROOT / ".pre-commit-hooks.yaml"
WORKFLOW = ROOT / ".github" / "workflows" / "aos-precommit-release.yml"


def _release_paths() -> list[str]:
    """The push-trigger path filter that decides whether a release is cut."""
    workflow = yaml.safe_load(WORKFLOW.read_text(encoding="utf-8"))
    # PyYAML resolves the bare `on:` key to True, so accept either spelling.
    triggers = workflow.get("on", workflow.get(True))
    return list(triggers["push"]["paths"])


def _hook_scripts() -> dict[str, str]:
    """Map hook id to the repo script its entry runs, for script-backed hooks."""
    hooks = yaml.safe_load(HOOKS.read_text(encoding="utf-8"))
    scripts = {}
    for hook in hooks:
        for token in str(hook.get("entry", "")).split():
            if token.startswith("scripts/"):
                scripts[hook["id"]] = token
    return scripts


def test_every_hook_script_triggers_a_release() -> None:
    paths = _release_paths()
    missing = {
        hook_id: script
        for hook_id, script in _hook_scripts().items()
        if script not in paths
    }
    assert not missing, (
        f"hook scripts absent from the aos-precommit-release paths filter: {missing}. "
        "Add each to .github/workflows/aos-precommit-release.yml so a fix to the "
        "script cuts a release (agentic-os#1148)."
    )


def test_the_hook_manifest_itself_triggers_a_release() -> None:
    """A new or retired hook id must move the tag consumers pin."""
    assert ".pre-commit-hooks.yaml" in _release_paths()
