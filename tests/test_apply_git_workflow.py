"""Tests for scripts/apply-git-workflow.py opt-out handling.

Regression cover for agentic-os#6894: the applier walked the whole workspace
and wrote a managed git-workflow block into `coilysiren/coilysiren`, which
carries `.agentic-os-ignore` and had deliberately never held one. The marker
documents itself as fail-closed with no override, and this writes agent-facing
doctrine into AGENTS.md, so the gate has to sit ahead of the read.
"""
from __future__ import annotations

import importlib.util
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "apply-git-workflow.py"


def _load_script():
    spec = importlib.util.spec_from_file_location("apply_git_workflow", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _repo(tmp_path: Path, name: str, agents: str = "# Repo\n") -> Path:
    repo = tmp_path / "coilysiren" / name
    repo.mkdir(parents=True)
    (repo / "AGENTS.md").write_text(agents, encoding="utf-8")
    return repo


def test_marker_skips_the_repo_before_anything_is_written(tmp_path: Path) -> None:
    script = _load_script()
    repo = _repo(tmp_path, "coilysiren")
    (repo / ".agentic-os-ignore").write_text("# opted out\n", encoding="utf-8")
    before = (repo / "AGENTS.md").read_text(encoding="utf-8")

    action, detail = script.apply_to_repo(repo, dry_run=False)

    assert action == "skip"
    assert ".agentic-os-ignore" in detail
    assert (repo / "AGENTS.md").read_text(encoding="utf-8") == before


def test_marker_skips_a_dry_run_too(tmp_path: Path) -> None:
    """A dry run reporting would-write on an opted-out repo is still wrong."""
    script = _load_script()
    repo = _repo(tmp_path, "coilysiren")
    (repo / ".agentic-os-ignore").write_text("", encoding="utf-8")

    action, _ = script.apply_to_repo(repo, dry_run=True)

    assert action == "skip"


def test_an_empty_marker_file_still_counts(tmp_path: Path) -> None:
    """The marker says contents are ignored and only existence matters."""
    script = _load_script()
    repo = _repo(tmp_path, "profile")
    (repo / ".agentic-os-ignore").touch()

    assert script.apply_to_repo(repo, dry_run=False)[0] == "skip"


def test_a_vendor_org_checkout_is_skipped(tmp_path: Path) -> None:
    """opted_out also covers vendor clones, and the applier inherits that."""
    script = _load_script()
    repo = tmp_path / "StrangeLoopGames" / "eco"
    repo.mkdir(parents=True)
    (repo / "AGENTS.md").write_text("# Vendor\n", encoding="utf-8")

    action, detail = script.apply_to_repo(repo, dry_run=False)

    assert action == "skip"
    assert "vendor org" in detail


def test_a_repo_without_the_marker_is_still_written(tmp_path: Path) -> None:
    """The negative control: the gate must not swallow ordinary repos."""
    script = _load_script()
    repo = _repo(tmp_path, "managed")

    action, _ = script.apply_to_repo(repo, dry_run=False)

    assert action == "wrote"
    assert (repo / "AGENTS.md").read_text(encoding="utf-8") != "# Repo\n"
