"""Tests for the shipped-hook catalog the applier and the audit both read."""
from __future__ import annotations

from pathlib import Path

from agentic_os import hook_catalog


def _hooks_file(tmp_path: Path, body: str) -> Path:
    path = tmp_path / ".pre-commit-hooks.yaml"
    path.write_text(body, encoding="utf-8")
    return path


CATALOG = """
- id: active-hook
  name: active
- id: manual-hook
  name: manual
  stages: [manual]
- id: mixed-hook
  name: mixed
  stages: [manual, pre-commit]
"""


def test_manual_only_ids_needs_every_stage_to_be_manual(tmp_path: Path) -> None:
    path = _hooks_file(tmp_path, CATALOG)
    assert hook_catalog.manual_only_ids(path) == {"manual-hook"}


# A manual-only id in the shipped set lands in every consumer block and never
# runs, which is how unresolved-placeholder-guard sat inert (#7628).
def test_inert_shipped_ids_flags_a_shipped_manual_hook(tmp_path, monkeypatch) -> None:
    path = _hooks_file(tmp_path, CATALOG)
    monkeypatch.setattr(
        hook_catalog, "DEFAULT_HOOK_IDS", ["active-hook", "manual-hook"], raising=True
    )
    assert hook_catalog.inert_shipped_ids(path) == ["manual-hook"]


def test_inert_shipped_ids_is_empty_when_every_shipped_hook_can_fire(
    tmp_path, monkeypatch
) -> None:
    path = _hooks_file(tmp_path, CATALOG)
    monkeypatch.setattr(
        hook_catalog, "DEFAULT_HOOK_IDS", ["active-hook", "mixed-hook"], raising=True
    )
    assert hook_catalog.inert_shipped_ids(path) == []


def test_undeclared_shipped_ids_flags_an_id_the_catalog_never_defines(
    tmp_path, monkeypatch
) -> None:
    path = _hooks_file(tmp_path, CATALOG)
    monkeypatch.setattr(
        hook_catalog, "DEFAULT_HOOK_IDS", ["active-hook", "ghost-hook"], raising=True
    )
    assert hook_catalog.undeclared_shipped_ids(path) == ["ghost-hook"]


def test_hook_ids_for_drops_the_repos_declared_skips() -> None:
    ids = hook_catalog.hook_ids_for("lore")
    assert "check-skills" not in ids
    assert "documentation-size" in ids


def test_hook_ids_for_drops_the_eco_skip() -> None:
    assert "code-comments" not in hook_catalog.hook_ids_for("eco-mods")
    assert "code-comments" in hook_catalog.hook_ids_for("agent-proxy")
