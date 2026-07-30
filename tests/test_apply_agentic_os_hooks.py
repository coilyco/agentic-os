"""Tests for scripts/apply-agentic-os-hooks.py repo resolution.

Regression cover for agentic-os#102: the workspace root must be driven by
$PROJECTS_ROOT, not a hardcoded ~/projects/coilysiren. On Windows the default
home/projects path is wrong (the workspace lives on another drive, e.g.
X:/projects), so a `--repo <name>` run there reported "not checked out
locally" and a full run skipped every repo. Setting PROJECTS_ROOT must fix it
end to end through the script's own main(), not just the config helpers.
"""
from __future__ import annotations

import importlib.util
import re
import subprocess
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "apply-agentic-os-hooks.py"
REPO_ROOT = SCRIPT.parent.parent
_HOOK_ID_RE = re.compile(r"^\s*- id:\s*(\S+)\s*$", re.MULTILINE)


def _hook_ids_at_ref(ref: str) -> set[str] | None:
    """Hook ids declared in .pre-commit-hooks.yaml at a git ref, or None if absent."""
    proc = subprocess.run(
        ["git", "show", f"{ref}:.pre-commit-hooks.yaml"],
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        return None
    return set(_HOOK_ID_RE.findall(proc.stdout))


def _load_script():
    spec = importlib.util.spec_from_file_location("apply_agentic_os_hooks", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _make_repo(path: Path) -> None:
    (path / ".git").mkdir(parents=True)
    (path / ".pre-commit-config.yaml").write_text("repos: []\n", encoding="utf-8")


def test_repo_found_via_projects_root_env(monkeypatch, tmp_path: Path) -> None:
    # Simulate the Windows case: workspace on a non-default root, not ~/projects.
    _make_repo(tmp_path / "coilysiren" / "atmosphere")
    monkeypatch.setenv("PROJECTS_ROOT", str(tmp_path))
    script = _load_script()
    assert script.main(["--repo", "atmosphere", "--dry-run"]) == 0


def test_repo_missing_without_env_override(monkeypatch, tmp_path: Path) -> None:
    # Same workspace, but PROJECTS_ROOT points elsewhere: the repo is unreachable,
    # which is exactly the failure the hardcoded root produced on Windows.
    _make_repo(tmp_path / "workspace" / "coilysiren" / "atmosphere")
    monkeypatch.setenv("PROJECTS_ROOT", str(tmp_path / "empty"))
    (tmp_path / "empty").mkdir()
    script = _load_script()
    assert script.main(["--repo", "atmosphere", "--dry-run"]) == 1


def test_full_run_spans_env_root(monkeypatch, tmp_path: Path, capsys) -> None:
    _make_repo(tmp_path / "coilyco-flight-deck" / "atmosphere")
    _make_repo(tmp_path / "coilysiren" / "warp")
    monkeypatch.setenv("PROJECTS_ROOT", str(tmp_path))
    script = _load_script()
    assert script.main(["--dry-run"]) == 0
    out = capsys.readouterr().out
    assert "atmosphere" in out
    assert "warp" in out


def test_default_rev_resolves_from_latest_tag(monkeypatch) -> None:
    """default_rev() picks the highest aos-precommit-v* tag."""
    script = _load_script()

    class _Out:
        stdout = (
            "aos-precommit-v0.61.0\n"
            "aos-precommit-v0.60.0\n"
            "aos-precommit-v0.5.0\n"
        )

    monkeypatch.setattr(
        script.subprocess, "run", lambda *a, **k: _Out(), raising=True
    )
    assert script.latest_release_tag() == "aos-precommit-v0.61.0"
    assert script.default_rev() == "aos-precommit-v0.61.0"


def test_default_rev_falls_back_without_tags(monkeypatch) -> None:
    """A tag-less (shallow) checkout falls back to FALLBACK_REV, not a crash."""
    script = _load_script()

    class _Empty:
        stdout = ""

    monkeypatch.setattr(
        script.subprocess, "run", lambda *a, **k: _Empty(), raising=True
    )
    assert script.latest_release_tag() is None
    assert script.default_rev() == script.FALLBACK_REV


def test_default_rev_falls_back_when_git_missing(monkeypatch) -> None:
    """git unavailable (OSError) resolves to the floor rather than raising."""
    script = _load_script()

    def _boom(*a, **k):
        raise OSError("git not found")

    monkeypatch.setattr(script.subprocess, "run", _boom, raising=True)
    assert script.latest_release_tag() is None
    assert script.default_rev() == script.FALLBACK_REV


def test_default_hook_ids_present_at_default_rev() -> None:
    """Regression cover for agentic-os#187: rollout drift between rev and hooks.

    The rollout pins rev=default_rev() but hooks=DEFAULT_HOOK_IDS. Adding a hook
    to the default list before a release pins it makes every consumer's
    pre-commit init fail ("<id> is not present in repository ... rev <REV>").
    Invariant: every default-rolled-out hook must exist at the resolved rev.
    """
    script = _load_script()
    rev = script.default_rev()
    ids_at_rev = _hook_ids_at_ref(rev)
    if ids_at_rev is None:
        pytest.skip(f"{rev} tag not available locally (shallow clone)")
    missing = [h for h in script.DEFAULT_HOOK_IDS if h not in ids_at_rev]
    assert not missing, (
        f"DEFAULT_HOOK_IDS reference hooks absent from {rev}: {missing}. "
        "Cut a release containing them, then ensure it is tagged (agentic-os#187)."
    )


def test_managed_block_includes_standard_hygiene_hooks() -> None:
    """The managed rollout block keeps the expanded hygiene suite together."""
    script = _load_script()
    block = script.managed_block("v9.9.9")
    for needle in (
        "actions-run-one-line",
        "source-doc-refs",
        "trailing-whitespace",
        "end-of-file-fixer",
        "check-added-large-files",
        "check-case-conflict",
        "check-illegal-windows-names",
        "mixed-line-ending",
        "check-json",
        "check-toml",
        "https://github.com/rhysd/actionlint",
        "files: ^\\.forgejo/workflows/.*\\.(ya?ml)$",
        "https://code.forgejo.org/forgejo/runner",
        "forgejo-runner-validate",
        "https://github.com/shellcheck-py/shellcheck-py",
        "https://github.com/crate-ci/typos",
        "args: []",
    ):
        assert needle in block


def test_refresh_protects_current_block_and_preserves_local_hooks(
    tmp_path: Path,
) -> None:
    """Managed-repo cleanup must not consume the current block's end marker."""
    script = _load_script()
    config = tmp_path / ".pre-commit-config.yaml"
    config.write_text(
        f"""\
repos:
{script.managed_block("v1.0.0")}
  - repo: local
    hooks:
      - id: repo-specific
        name: repository-specific hook
        entry: true
        language: system
  - repo: {script.TYPOS_REPO_URL}
    rev: v0.1.0
    hooks:
      - id: typos
""",
        encoding="utf-8",
    )

    status, removed = script.upsert_managed_block(config, "v2.0.0")
    rendered = config.read_text(encoding="utf-8")

    assert status == "updated"
    assert removed == 1
    assert rendered.count(script.BEGIN_MARKER) == 1
    assert rendered.count(script.END_MARKER) == 1
    assert "rev: v2.0.0" in rendered
    assert "repo-specific" in rendered
    assert "rev: v0.1.0" not in rendered
