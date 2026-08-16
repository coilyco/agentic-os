"""Tests for build output staying invisible to tree-walking hooks.

A gitignored bake was being read as the consuming repository's own content, so
`compose-bundles` followed by the commit gate failed on skills the repository
does not own. See sirens-echo#800 and docs/build-output-is-not-content.md.
"""
from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

from agentic_os import config
from agentic_os.pre_commit import check_dead_links as cdl
from agentic_os.pre_commit import check_documentation_layout as cdocs

# A misplaced skill under a path documentation-layout rejects, carrying a
# relative link that resolves in the catalogue and not in a bake.
BAKED = "agent/bundles/qa/content/skills/roster/role-qa/SKILL.md"
BAKED_BODY = "See [Shows](../personal-preference-shows/COMPOSED.md).\n"


def _write(root: Path, rel: str, body: str) -> None:
    path = root / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(body, encoding="utf-8")


def _git(root: Path, *args: str) -> None:
    subprocess.run(
        ["git", *args],
        cwd=root,
        check=True,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )


def _checkout(root: Path, ignore: str | None) -> None:
    """A real git checkout, because the answer here comes from git itself."""
    _git(root, "init", "-q")
    _write(root, "README.md", "# Repo\n")
    # Every repo declares a band, a fixture repo included.
    _write(
        root,
        "pyproject.toml",
        '[tool.agentic-os.documentation-layout]\nband = "small"\n',
    )
    if ignore is not None:
        _write(root, ".gitignore", f"{ignore}\n")
    _write(root, BAKED, BAKED_BODY)
    _git(root, "add", "-A")
    _git(root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "seed")


@pytest.fixture(autouse=True)
def _fresh_cache() -> None:
    config.reset_build_output_cache()


def _run_links(monkeypatch: pytest.MonkeyPatch, root: Path) -> int:
    monkeypatch.setattr(cdl, "REPO_ROOT", root)
    monkeypatch.setattr(config, "REPO_ROOT", root)
    return cdl.main(["check-dead-links"])


def _run_layout(monkeypatch: pytest.MonkeyPatch, root: Path) -> int:
    monkeypatch.setattr(cdocs, "REPO_ROOT", root)
    monkeypatch.setattr(config, "REPO_ROOT", root)
    return cdocs.main()


def test_a_gitignored_bake_is_not_this_repos_markdown(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    _checkout(tmp_path, ignore="agent/bundles/")
    assert _run_layout(monkeypatch, tmp_path) == 0


def test_a_gitignored_bake_carries_no_dead_links(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    _checkout(tmp_path, ignore="agent/bundles/")
    assert _run_links(monkeypatch, tmp_path) == 0


# The controls. Without these the two above pass on a hook that checks nothing.


def test_the_same_tree_tracked_still_fails_layout(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    _checkout(tmp_path, ignore=None)
    assert _run_layout(monkeypatch, tmp_path) == 1


def test_the_same_tree_tracked_still_fails_links(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    _checkout(tmp_path, ignore=None)
    assert _run_links(monkeypatch, tmp_path) == 1


def test_a_tree_git_cannot_read_is_checked_as_before(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # No checkout at all. Failing open here would silently stop every hook on a
    # tarball, a vendored copy, or a machine with no git.
    _write(tmp_path, "README.md", "# Repo\n")
    _write(tmp_path, BAKED, BAKED_BODY)
    assert config.is_build_output(BAKED, tmp_path) is False
    assert _run_layout(monkeypatch, tmp_path) == 1


def test_a_directory_holding_tracked_files_is_content(tmp_path: Path) -> None:
    # docs/ flatness reads directories, and a directory is never in git's file
    # list. Reading one as build output would retire that rule.
    _checkout(tmp_path, ignore="agent/bundles/")
    _write(tmp_path, "docs/nested/x.md", "")
    _git(tmp_path, "add", "-A")
    config.reset_build_output_cache()
    assert config.is_build_output("docs/nested", tmp_path) is False
    assert config.is_build_output("agent/bundles/qa", tmp_path) is True


def test_an_untracked_source_file_is_still_content(tmp_path: Path) -> None:
    # Untracked but not ignored is a file the author has not staged yet, not
    # build output, and a hook that skipped it would report a clean tree.
    _checkout(tmp_path, ignore="agent/bundles/")
    _write(tmp_path, "docs/new.md", "")
    config.reset_build_output_cache()
    assert config.is_build_output("docs/new.md", tmp_path) is False
