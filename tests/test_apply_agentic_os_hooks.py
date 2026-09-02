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
import yaml

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
        "args: [--force-exclude]",
    ):
        assert needle in block


def _write_actionlint_config(repo_dir: Path) -> None:
    (repo_dir / ".github").mkdir(parents=True, exist_ok=True)
    (repo_dir / ".github" / "actionlint.yaml").write_text(
        "self-hosted-runner:\n  labels:\n    - docker\n", encoding="utf-8"
    )


def _actionlint_hook(rendered: str) -> dict:
    config = yaml.safe_load(rendered)
    for repo in config["repos"]:
        if repo["repo"] == "https://github.com/rhysd/actionlint":
            return repo["hooks"][0]
    raise AssertionError("no actionlint repo in the rendered block")


def test_actionlint_config_flag_follows_the_consumer(tmp_path: Path) -> None:
    """agentic-os#984: a Forgejo-only repo needs the flag to see its labels.

    actionlint keys project detection off .github/workflows, so a repo whose
    workflows live in .forgejo/workflows never auto-discovers its config and
    reports a declared self-hosted runner label as unknown. The flag is not
    emitted unconditionally: actionlint exits non-zero on a config path it
    cannot read, which would break every consumer that ships no config.
    """
    script = _load_script()
    without = _actionlint_hook(f"repos:\n{script.managed_block('v9.9.9', repo_dir=tmp_path)}")
    assert "args" not in without

    _write_actionlint_config(tmp_path)
    with_config = _actionlint_hook(
        f"repos:\n{script.managed_block('v9.9.9', repo_dir=tmp_path)}"
    )
    assert with_config["args"] == ["-config-file", ".github/actionlint.yaml"]
    assert with_config["files"] == r"^\.forgejo/workflows/.*\.(ya?ml)$"


def test_apply_carries_the_actionlint_config_flag(tmp_path: Path) -> None:
    # The consumer dir is derived from the config path, so a real apply, not
    # just the template helper, has to carry the flag.
    script = _load_script()
    _write_actionlint_config(tmp_path)
    config = tmp_path / ".pre-commit-config.yaml"

    assert script.upsert_managed_block(config, "v2.0.0")[0] == "created"
    created = _actionlint_hook(config.read_text(encoding="utf-8"))
    assert created["args"] == ["-config-file", ".github/actionlint.yaml"]

    # The refresh path rewrites the block from scratch, so it has to keep it.
    script.upsert_managed_block(config, "v2.0.0")
    refreshed = _actionlint_hook(config.read_text(encoding="utf-8"))
    assert refreshed["args"] == ["-config-file", ".github/actionlint.yaml"]


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


def test_vendored_trees_exclude_only_the_rewriting_hooks(tmp_path: Path) -> None:
    """A vendored tree opts out of the fixers, not the reporting hooks."""
    script = _load_script()
    (tmp_path / "pyproject.toml").write_text(
        '[tool.agentic-os.managed-hooks]\nvendored = ["mods/Mods/", "vendor/sdk"]\n',
        encoding="utf-8",
    )

    block = script.managed_block("v1.0.0", ["catalog-trifecta"], tmp_path)
    expected = "        exclude: ^(mods/Mods/|vendor/sdk/)"

    for hook_id in ("trailing-whitespace", "end-of-file-fixer", "mixed-line-ending"):
        assert f"      - id: {hook_id}\n{expected}" in block

    for hook_id in ("check-json", "check-toml", "check-case-conflict"):
        assert f"      - id: {hook_id}\n{expected}" not in block


def test_no_vendored_declaration_leaves_the_hooks_bare(tmp_path: Path) -> None:
    script = _load_script()
    block = script.managed_block("v1.0.0", ["catalog-trifecta"], tmp_path)
    assert "exclude: ^(" not in block
    assert "      - id: trailing-whitespace\n      - id: end-of-file-fixer" in block


def test_check_json_always_skips_vscode_jsonc(tmp_path: Path) -> None:
    """VS Code documents launch/tasks/settings.json as JSONC, so it never parses."""
    script = _load_script()
    block = script.managed_block("v1.0.0", ["catalog-trifecta"], tmp_path)
    expected = "      - id: check-json" + chr(10) + "        exclude: (^|/)" + chr(92) + ".vscode/"
    assert expected in block


def test_gitattributes_pins_the_working_tree_not_just_text_auto() -> None:
    """text=auto alone still checks out CRLF under core.autocrlf=true."""
    script = _load_script()
    block = script.gitattributes_block(None)
    assert "* text=auto eol=lf" in block
    assert "*.bat text eol=crlf" in block
    assert "*.cmd text eol=crlf" in block
    assert block.startswith(script.BEGIN_MARKER)
    assert script.END_MARKER in block


def test_gitattributes_keeps_vendored_trees_byte_exact(tmp_path: Path) -> None:
    script = _load_script()
    (tmp_path / "pyproject.toml").write_text(
        '[tool.agentic-os.managed-hooks]\nvendored = ["mods/Mods/", "vendor/sdk"]\n',
        encoding="utf-8",
    )
    block = script.gitattributes_block(tmp_path)
    assert "mods/Mods/** -text" in block
    assert "vendor/sdk/** -text" in block


def test_gitattributes_block_goes_first_and_keeps_local_rules(tmp_path: Path) -> None:
    """Git takes the last matching pattern, so a general rule below LFS lines wins."""
    script = _load_script()
    path = tmp_path / ".gitattributes"
    path.write_text("*.png filter=lfs diff=lfs merge=lfs -text" + chr(92) + "n", encoding="utf-8")

    assert script.ensure_gitattributes(tmp_path) == "prepended"
    text = path.read_text(encoding="utf-8")
    assert text.startswith(script.BEGIN_MARKER)
    assert text.index("* text=auto eol=lf") < text.index("*.png filter=lfs")

    assert script.ensure_gitattributes(tmp_path) is None
    assert path.read_text(encoding="utf-8") == text


def test_gitattributes_created_where_a_repo_has_none(tmp_path: Path) -> None:
    script = _load_script()
    assert script.ensure_gitattributes(tmp_path) == "created"
    assert (tmp_path / ".gitattributes").read_text(encoding="utf-8").startswith(
        script.BEGIN_MARKER
    )


def test_vendor_org_checkouts_are_never_written_to(tmp_path: Path) -> None:
    """An upstream checkout owns its own eol rules; a managed block would fight them."""
    script = _load_script()
    repo = tmp_path / "StrangeLoopGames" / "Eco"
    repo.mkdir(parents=True)
    (repo / ".git").mkdir()

    status, detail = script.apply_to_repo(repo, "v1.0.0", dry_run=False)
    assert status == "skipped"
    assert "vendor org" in detail
    assert not (repo / ".gitattributes").exists()
    assert not (repo / ".pre-commit-config.yaml").exists()


def test_a_created_config_is_unchanged_on_its_next_refresh(tmp_path: Path) -> None:
    # Two renderings of the blank line after `repos:` meant a fresh config
    # reported `updated` once and settled only on run three. agentic-os#985.
    script = _load_script()
    config = tmp_path / ".pre-commit-config.yaml"
    statuses = [script.upsert_managed_block(config, "v2.0.0")[0] for _ in range(3)]

    assert statuses == ["created", "unchanged", "unchanged"]


def test_a_hand_written_preamble_keeps_its_blank_line(tmp_path: Path) -> None:
    # The separator divides a repo's own hooks from the managed block. Only
    # the bare document opener has nothing to divide.
    script = _load_script()
    block = script.managed_block("v2.0.0")

    assert script.render_config("repos:", block) == "repos:\n" + block
    local = "repos:\n  - repo: local\n    hooks:\n      - id: mine"
    assert script.render_config(local, block) == local + "\n\n" + block


def test_this_repos_own_typos_entry_matches_the_block_it_ships() -> None:
    # The repo that authors the block did not run it, so its own config drifted
    # to `args: []` and its .typos.toml path excludes went inert. #1186.
    script = _load_script()
    generated = script.managed_block("v2.0.0")
    own = (Path(__file__).resolve().parent.parent / ".pre-commit-config.yaml").read_text(
        encoding="utf-8"
    )

    assert "args: [--force-exclude]" in generated
    typos_entry = own.split("- id: typos", 1)[1].split("- repo:", 1)[0]
    assert "--force-exclude" in typos_entry, typos_entry


def test_the_source_repo_keeps_its_config_and_still_gets_hooks(tmp_path: Path) -> None:
    # The self-skip protected the hand-maintained config and skipped the hook
    # install too, leaving pr-guard's author the one repo not running it. #1192.
    script = _load_script()
    repo = tmp_path / "coilyco-flight-deck" / "agentic-os"
    _make_repo(repo)
    before = (repo / ".pre-commit-config.yaml").read_text()

    status, detail = script.apply_to_repo(repo, "v1.0.0", dry_run=False)

    assert status == "applied"
    assert "config untouched" in detail
    assert (repo / ".pre-commit-config.yaml").read_text() == before
    assert not (repo / ".gitattributes").exists()


def test_the_source_repo_dry_run_says_hooks_only(tmp_path: Path) -> None:
    script = _load_script()
    repo = tmp_path / "coilyco-flight-deck" / "agentic-os"
    _make_repo(repo)

    status, detail = script.apply_to_repo(repo, "v1.0.0", dry_run=True)

    assert status == "dryrun"
    assert "install hooks only" in detail


def _own_config() -> dict:
    return yaml.safe_load((REPO_ROOT / ".pre-commit-config.yaml").read_text())


def _own_third_party() -> dict[str, dict]:
    return {r["repo"]: r for r in _own_config()["repos"] if r.get("repo") != "local"}


def test_the_source_repo_pins_every_shared_hook_repo_like_its_consumers() -> None:
    # #1199: agentic-os generates a consumer's config and hand-maintains its
    # own, so the two drifted twice (#1186, #1181) with nothing to notice.
    script = _load_script()
    mine = _own_third_party()
    expected = {
        script.PRECOMMIT_HOOKS_REPO_URL: script.PRECOMMIT_HOOKS_REV,
        script.ACTIONLINT_REPO_URL: script.ACTIONLINT_REV,
        script.FORGEJO_RUNNER_REPO_URL: script.FORGEJO_RUNNER_REV,
        script.SHELLCHECK_REPO_URL: script.SHELLCHECK_REV,
        script.TYPOS_REPO_URL: script.TYPOS_REV,
    }
    for url, rev in expected.items():
        assert url in mine, (
            f"the generated block ships {url} to every consumer and this repo's "
            f"own config does not carry it."
        )
        assert mine[url]["rev"] == rev, (
            f"{url}: this repo pins {mine[url]['rev']}, the generator sends "
            f"consumers {rev}. Bump both or neither."
        )
