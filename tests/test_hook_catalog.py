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
    assert "repo-pointer-skills" not in ids
    assert "documentation-size" in ids


# lore declared a 4000-char entry cap that nothing evaluated until this hook
# shipped there alongside its categories.yaml (teable:coilyco-bridge/lore#7753).
def test_hook_ids_for_ships_check_skills_to_lore() -> None:
    assert "check-skills" in hook_catalog.hook_ids_for("lore")


def test_hook_ids_for_drops_the_eco_skip() -> None:
    assert "code-comments" not in hook_catalog.hook_ids_for("eco-mods")
    assert "code-comments" in hook_catalog.hook_ids_for("agent-proxy")


# The applier refuses to write into a vendor org, so the audit must expect
# nothing of one rather than reporting it missing every hook (#7635).
def test_opted_out_covers_a_vendor_org() -> None:
    out, why = hook_catalog.opted_out(Path("/p/StrangeLoopGames/Eco"))
    assert out and "vendor org" in why
    assert not hook_catalog.opted_out(Path("/p/coilyco-bridge/lore"))[0]


# A marker file is the explicit opt-out, and the audit honoured neither until
# #7638, so a repo that asked to be left alone was its loudest failure.
def test_opted_out_covers_the_ignore_marker(tmp_path: Path) -> None:
    repo = tmp_path / "org" / "quiet-repo"
    repo.mkdir(parents=True)
    assert not hook_catalog.opted_out(repo)[0]
    (repo / hook_catalog.IGNORE_MARKER).write_text("")
    out, why = hook_catalog.opted_out(repo)
    assert out and hook_catalog.IGNORE_MARKER in why


# Merging the two predicates would silence a real finding: the source repo takes
# no block and still owes coverage, which is how #7634 surfaced.
def test_source_repo_takes_no_block_but_is_not_exempt() -> None:
    source = Path("/p/coilyco-flight-deck/agentic-os")
    assert not hook_catalog.ships_to(source)
    assert not hook_catalog.opted_out(source)[0]


def test_an_ordinary_repo_takes_the_block() -> None:
    assert hook_catalog.ships_to(Path("/p/coilyco-bridge/lore"))


# A bare "eco" prefix also caught ecommerce-shaped names.
def test_eco_skip_needs_the_hyphen() -> None:
    assert "code-comments" not in hook_catalog.hook_ids_for("eco-app")
    assert "code-comments" in hook_catalog.hook_ids_for("ecommerce-storefront")


def _skill_repo(root: Path, *, spec: bool, entry: bool = True) -> Path:
    skills = root / ".agents" / "skills" / "coding-go"
    skills.mkdir(parents=True)
    if entry:
        (skills / "SKILL.md").write_text("x", encoding="utf-8")
    if spec:
        (root / ".agents" / "skills" / "categories.yaml").write_text("x", encoding="utf-8")
    return root


# The failure this catches runs, passes, and evaluates nothing, so it is
# invisible to every surface that reads config rather than the filesystem.
def test_a_hook_with_no_spec_to_read_is_unarmed(tmp_path: Path) -> None:
    repo = _skill_repo(tmp_path, spec=False)

    found = hook_catalog.unarmed_spec_hooks(repo, {"check-skills"})

    assert found == ["check-skills: no .agents/skills/categories.yaml"]


def test_a_hook_with_its_spec_is_armed(tmp_path: Path) -> None:
    repo = _skill_repo(tmp_path, spec=True)

    assert hook_catalog.unarmed_spec_hooks(repo, {"check-skills"}) == []


def test_a_repo_that_does_not_run_the_hook_is_not_reported(tmp_path: Path) -> None:
    repo = _skill_repo(tmp_path, spec=False)

    assert hook_catalog.unarmed_spec_hooks(repo, set()) == []


def test_a_repo_with_no_skills_at_all_is_not_reported(tmp_path: Path) -> None:
    # Nothing to check is not the same as checking nothing.
    assert hook_catalog.unarmed_spec_hooks(tmp_path, {"check-skills"}) == []


def test_an_empty_skills_dir_is_not_reported(tmp_path: Path) -> None:
    repo = _skill_repo(tmp_path, spec=False, entry=False)

    assert hook_catalog.unarmed_spec_hooks(repo, {"check-skills"}) == []


# detect_skills_dir takes the first root that exists, so the audit has to agree
# with it or it reports against a directory the hook never reads.
def test_the_first_matching_root_decides(tmp_path: Path) -> None:
    repo = _skill_repo(tmp_path, spec=True)
    legacy = repo / "skills" / "coding-go"
    legacy.mkdir(parents=True)
    (legacy / "SKILL.md").write_text("x", encoding="utf-8")

    assert hook_catalog.unarmed_spec_hooks(repo, {"check-skills"}) == []
