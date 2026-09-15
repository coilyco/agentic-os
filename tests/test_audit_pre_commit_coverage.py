"""Tests for the coverage audit's signal: what it expects, and what it can see."""
from __future__ import annotations

import importlib.util
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "audit-pre-commit-coverage.py"


def _load_script():
    spec = importlib.util.spec_from_file_location("audit_pre_commit_coverage", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


# agentic-os wires every validator it defines as `repo: local`, so the
# upstream-ref-only read reported the authoring repo as missing all of them.
def test_referenced_hook_ids_counts_locally_wired_hooks() -> None:
    audit = _load_script()
    config = """
repos:
  - repo: local
    hooks:
      - id: code-comments
      - id: brand-case
"""
    assert audit.referenced_hook_ids(config) == {"code-comments", "brand-case"}


def test_referenced_hook_ids_still_counts_the_upstream_ref() -> None:
    audit = _load_script()
    config = """
repos:
  - repo: https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os
    rev: aos-precommit-v0.92.0
    hooks:
      - id: code-comments
"""
    assert audit.referenced_hook_ids(config) == {"code-comments"}


# Only agentic-os and local count. A third party shipping a colliding id must
# not satisfy the audit, or coverage reads as met by an unrelated hook.
def test_referenced_hook_ids_ignores_an_unrelated_upstream() -> None:
    audit = _load_script()
    config = """
repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    hooks:
      - id: code-comments
"""
    assert audit.referenced_hook_ids(config) == set()


# The expected set is what the applier would write for that repo, so a
# deliberate per-repo skip must not read as a missing hook.
def test_audit_honours_per_repo_skips() -> None:
    audit = _load_script()
    from agentic_os import hook_catalog

    expected = hook_catalog.hook_ids_for("lore")
    config = "repos:\n  - repo: local\n    hooks:\n" + "".join(
        f"      - id: {h}\n" for h in expected
    )
    result = audit.audit_config("lore", config, expected)
    assert result["status"] == "ok", result
    assert "repo-pointer-skills" not in expected


# The audit has to see this from the filesystem: the config that runs the hook
# looks identical whether or not a spec exists for it to read.
def test_the_audit_reports_a_hook_with_no_spec(tmp_path, monkeypatch, capsys) -> None:
    audit = _load_script()
    from agentic_os import hook_catalog

    repo = tmp_path / "lore"
    (repo / ".agents" / "skills" / "lore-x").mkdir(parents=True)
    (repo / ".agents" / "skills" / "lore-x" / "SKILL.md").write_text("x")
    ids = hook_catalog.hook_ids_for("lore")
    (repo / ".pre-commit-config.yaml").write_text(
        "repos:\n  - repo: local\n    hooks:\n"
        + "".join(f"      - id: {h}\n" for h in ids)
    )

    monkeypatch.setattr(audit.cfg, "iter_workspace_repos", lambda: [repo])
    monkeypatch.setattr(audit, "_print_freshness", lambda dirs: None)

    code = audit.main([])
    out = capsys.readouterr().out

    assert "== unarmed (1) ==" in out
    assert "check-skills: no .agents/skills/categories.yaml" in out
    assert code == 1


def test_the_audit_is_quiet_once_the_spec_lands(tmp_path, monkeypatch, capsys) -> None:
    audit = _load_script()
    from agentic_os import hook_catalog

    repo = tmp_path / "lore"
    (repo / ".agents" / "skills" / "lore-x").mkdir(parents=True)
    (repo / ".agents" / "skills" / "lore-x" / "SKILL.md").write_text("x")
    (repo / ".agents" / "skills" / "categories.yaml").write_text("x")
    ids = hook_catalog.hook_ids_for("lore")
    (repo / ".pre-commit-config.yaml").write_text(
        "repos:\n  - repo: local\n    hooks:\n"
        + "".join(f"      - id: {h}\n" for h in ids)
    )

    monkeypatch.setattr(audit.cfg, "iter_workspace_repos", lambda: [repo])
    monkeypatch.setattr(audit, "_print_freshness", lambda dirs: None)

    audit.main([])

    assert "unarmed" not in capsys.readouterr().out
