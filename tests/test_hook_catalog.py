"""Tests for the shipped-hook catalog the applier and the audit both read."""
from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

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


def test_missing_pre_push_ids_flags_a_commit_only_hook(tmp_path, monkeypatch) -> None:
    path = _hooks_file(tmp_path, CATALOG)
    monkeypatch.setattr(
        hook_catalog, "PRE_PUSH_HOOK_IDS", ["active-hook", "ghost-hook"], raising=True
    )
    assert hook_catalog.missing_pre_push_ids(path) == ["active-hook", "ghost-hook"]


def test_missing_pre_push_ids_is_empty_once_declared(tmp_path, monkeypatch) -> None:
    path = _hooks_file(tmp_path, CATALOG + "- id: both\n  stages: [pre-commit, pre-push]\n")
    monkeypatch.setattr(hook_catalog, "PRE_PUSH_HOOK_IDS", ["both"], raising=True)
    assert hook_catalog.missing_pre_push_ids(path) == []


# A rebase can land a red main past a commit-only stage (COI-1052), so the
# shipped catalog must declare pre-push on every cheap whole-repo validator.
def test_shipped_catalog_declares_pre_push_for_the_cheap_validators() -> None:
    assert hook_catalog.missing_pre_push_ids() == []


def test_hook_ids_for_drops_the_repos_declared_skips() -> None:
    ids = hook_catalog.hook_ids_for("coilyco/lore")
    assert "repo-pointer-skills" not in ids
    assert "documentation-size" in ids


# lore declared a 4000-char entry cap that nothing evaluated until this hook
# shipped there alongside its categories.yaml (teable:coilyco-bridge/lore#7753).
def test_hook_ids_for_ships_check_skills_to_lore() -> None:
    assert "check-skills" in hook_catalog.hook_ids_for("coilyco/lore")


def test_hook_ids_for_drops_the_eco_skip() -> None:
    assert "code-comments" not in hook_catalog.hook_ids_for("coilyco/eco-mods")
    assert "code-comments" in hook_catalog.hook_ids_for("coilyco/agent-proxy")


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
    assert "code-comments" not in hook_catalog.hook_ids_for("coilyco/eco-app")
    assert "code-comments" in hook_catalog.hook_ids_for("coilyco/ecommerce-storefront")


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


# repo-pointer-skills owns a generated pointer, so a tree of nothing else has
# no check going unperformed and a finding here would never be acted on.
def test_a_tree_of_only_generated_pointers_is_not_reported(tmp_path: Path) -> None:
    pointer = tmp_path / ".agents" / "skills" / "repo-eco-app"
    pointer.mkdir(parents=True)
    (pointer / "SKILL.md").write_text("x", encoding="utf-8")

    assert hook_catalog.unarmed_spec_hooks(tmp_path, {"check-skills"}) == []


def test_one_hand_written_skill_beside_a_pointer_is_reported(tmp_path: Path) -> None:
    skills = tmp_path / ".agents" / "skills"
    for name in ("repo-eco-app", "coding-go"):
        (skills / name).mkdir(parents=True)
        (skills / name / "SKILL.md").write_text("x", encoding="utf-8")

    assert hook_catalog.unarmed_spec_hooks(tmp_path, {"check-skills"}) == [
        "check-skills: no .agents/skills/categories.yaml"
    ]


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


def _checkout(path: Path, origin: str | None) -> Path:
    path.mkdir(parents=True)
    subprocess.run(["git", "init", "-q", str(path)], check=True)
    if origin:
        subprocess.run(["git", "-C", str(path), "remote", "add", "origin", origin], check=True)
    return path


# One basename under two owners is two repos, and a skip belongs to one of them
# (agentic-os#7635). The directory pair was the same for both only by accident.
def test_two_repos_sharing_a_basename_get_different_skips(monkeypatch) -> None:
    monkeypatch.setitem(hook_catalog.PER_REPO_HOOK_SKIPS, "coilyco/website", {"typos-skip"})
    monkeypatch.setattr(
        hook_catalog, "DEFAULT_HOOK_IDS", ["code-comments", "typos-skip"], raising=True
    )
    assert hook_catalog.hook_ids_for("coilyco/website") == ["code-comments"]
    assert hook_catalog.hook_ids_for("coilysiren/website") == ["code-comments", "typos-skip"]


def test_a_bare_basename_is_refused_rather_than_shipping_every_hook() -> None:
    with pytest.raises(ValueError, match="<owner>/<repo>"):
        hook_catalog.hook_ids_for("lore")


def test_the_eco_skip_is_owner_qualified() -> None:
    assert "code-comments" not in hook_catalog.hook_ids_for("coilyco/eco-ops")
    assert "code-comments" in hook_catalog.hook_ids_for("someone-else/eco-app")


@pytest.mark.parametrize(
    "origin",
    [
        "https://forgejo.coilysiren.me/coilyco/lore.git",
        "ssh://git@localhost:2222/coilyco/lore.git",
        "git@github.com:coilyco/lore.git",
        "https://forgejo.coilysiren.me/coilyco/lore",
    ],
)
def test_repo_key_reads_the_owner_from_origin(tmp_path: Path, origin: str) -> None:
    assert hook_catalog.repo_key(_checkout(tmp_path / "lore", origin)) == "coilyco/lore"


# A retired org dir holds a checkout whose remote owner is the current one, so
# the parent name would key the same repo differently on each host.
def test_repo_key_ignores_which_org_dir_the_checkout_sits_in(tmp_path: Path) -> None:
    origin = "https://forgejo.coilysiren.me/coilyco/lore.git"
    one = _checkout(tmp_path / "coilyco-bridge" / "lore", origin)
    two = _checkout(tmp_path / "coilyco" / "lore", origin)
    assert hook_catalog.repo_key(one) == hook_catalog.repo_key(two) == "coilyco/lore"
    assert "repo-pointer-skills" not in hook_catalog.hook_ids_for(hook_catalog.repo_key(one))


def test_two_checkouts_of_one_basename_key_apart_by_origin(tmp_path: Path) -> None:
    a = _checkout(tmp_path / "a" / "website", "https://h.example/coilyco/website.git")
    b = _checkout(tmp_path / "b" / "website", "https://h.example/coilysiren/website.git")
    assert hook_catalog.repo_key(a) != hook_catalog.repo_key(b)


def test_repo_key_falls_back_to_the_directory_pair_without_an_origin(tmp_path: Path) -> None:
    assert hook_catalog.repo_key(_checkout(tmp_path / "org" / "bare", None)) == "org/bare"
