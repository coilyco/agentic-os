"""Tests for agentic_os.config: per-repo exclude and enabled flags."""
from __future__ import annotations

from pathlib import Path

import pytest

from agentic_os.config import (
    get_int_option,
    is_enabled,
    is_excluded,
    WorkspaceRootMissing,
    iter_workspace_repos,
    load_excludes,
    projects_root,
)


@pytest.fixture
def repo(tmp_path: Path) -> Path:
    return tmp_path


def write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


# ---------- load_excludes ----------

def test_no_config_returns_empty(repo: Path) -> None:
    assert load_excludes("documentation-layout", repo) == []


def test_pyproject_excludes(repo: Path) -> None:
    write(repo / "pyproject.toml", """
[tool.agentic-os.documentation-layout]
excludes = ["src/pages/", "**/generated/*.md"]
""")
    assert load_excludes("documentation-layout", repo) == [
        "src/pages/",
        "**/generated/*.md",
    ]


def test_agentic_os_toml_excludes(repo: Path) -> None:
    write(repo / ".agentic-os.toml", """
[documentation-layout]
excludes = ["src/pages/**"]
""")
    assert load_excludes("documentation-layout", repo) == ["src/pages/**"]


def test_pyproject_wins_over_agentic_os_toml(repo: Path) -> None:
    write(repo / "pyproject.toml", """
[tool.agentic-os.documentation-layout]
excludes = ["from-pyproject/"]
""")
    write(repo / ".agentic-os.toml", """
[documentation-layout]
excludes = ["from-toml/"]
""")
    assert load_excludes("documentation-layout", repo) == ["from-pyproject/"]


def test_other_hook_unaffected(repo: Path) -> None:
    write(repo / "pyproject.toml", """
[tool.agentic-os.documentation-layout]
excludes = ["src/pages/"]
""")
    assert load_excludes("code-comments", repo) == []


def test_malformed_excludes_returns_empty(repo: Path) -> None:
    write(repo / "pyproject.toml", """
[tool.agentic-os.documentation-layout]
excludes = "not-a-list"
""")
    assert load_excludes("documentation-layout", repo) == []


def test_malformed_toml_returns_empty(repo: Path) -> None:
    write(repo / "pyproject.toml", "this is not valid toml = {{{")
    assert load_excludes("documentation-layout", repo) == []


# ---------- is_enabled ----------

def test_enabled_default_true(repo: Path) -> None:
    assert is_enabled("documentation-layout", repo) is True


def test_enabled_false(repo: Path) -> None:
    write(repo / "pyproject.toml", """
[tool.agentic-os.catalog-trifecta]
enabled = false
""")
    assert is_enabled("catalog-trifecta", repo) is False


def test_enabled_explicit_true(repo: Path) -> None:
    write(repo / "pyproject.toml", """
[tool.agentic-os.documentation-layout]
enabled = true
""")
    assert is_enabled("documentation-layout", repo) is True


def test_enabled_via_agentic_os_toml(repo: Path) -> None:
    write(repo / ".agentic-os.toml", """
[catalog-trifecta]
enabled = false
""")
    assert is_enabled("catalog-trifecta", repo) is False


# ---------- get_int_option ----------

def test_int_option_default_when_unset(repo: Path) -> None:
    assert get_int_option("documentation-layout", "agents_md_max_chars", 4000, repo) == 4000


def test_int_option_reads_pyproject(repo: Path) -> None:
    write(repo / "pyproject.toml", """
[tool.agentic-os.documentation-layout]
agents_md_max_lines = 160
agents_md_max_chars = 12000
""")
    assert get_int_option("documentation-layout", "agents_md_max_lines", 80, repo) == 160
    assert get_int_option("documentation-layout", "agents_md_max_chars", 4000, repo) == 12000


def test_int_option_reads_readme_opt_up(repo: Path) -> None:
    # The root README opts past the trifecta cap the same way AGENTS.md does.
    write(repo / "pyproject.toml", """
[tool.agentic-os.documentation-layout]
readme_max_lines = 400
readme_max_chars = 30000
""")
    assert get_int_option("documentation-layout", "readme_max_lines", 160, repo) == 400
    assert get_int_option("documentation-layout", "readme_max_chars", 12500, repo) == 30000


def test_int_option_rejects_non_int(repo: Path) -> None:
    write(repo / "pyproject.toml", """
[tool.agentic-os.documentation-layout]
agents_md_max_chars = "lots"
""")
    assert get_int_option("documentation-layout", "agents_md_max_chars", 4000, repo) == 4000


def test_int_option_rejects_bool(repo: Path) -> None:
    write(repo / "pyproject.toml", """
[tool.agentic-os.documentation-layout]
agents_md_max_chars = true
""")
    assert get_int_option("documentation-layout", "agents_md_max_chars", 4000, repo) == 4000


def test_int_option_scoped_to_hook(repo: Path) -> None:
    write(repo / "pyproject.toml", """
[tool.agentic-os.documentation-layout]
agents_md_max_chars = 12000
""")
    assert get_int_option("code-comments", "agents_md_max_chars", 4000, repo) == 4000


# ---------- is_excluded ----------

@pytest.mark.parametrize("path, patterns, expected", [
    ("src/pages/foo.md", ["src/pages/"], True),
    ("src/pages/posts/bar.md", ["src/pages/"], True),
    ("src/components/foo.tsx", ["src/pages/"], False),
    ("src/pages/foo.md", ["src/pages/**"], True),
    ("src/pages", ["src/pages/**"], True),
    ("other.md", ["src/pages/**"], False),
    # A pattern with a slash is anchored to the repo root.
    ("docs/foo.md", ["docs/*.md"], True),
    ("docs/sub/foo.md", ["docs/*.md"], False),
    ("a/b/c.md", ["**/c.md"], True),
    # A slash-less pattern matches the basename at any depth (gitignore-style):
    # one wildcard covers a generated file wherever it is emitted.
    ("README.md", ["*.md"], True),
    ("nested/README.md", ["*.md"], True),
    ("docs/generated.aws.md", ["generated.*.md"], True),
    ("cmd/generated/generated.aws.md", ["generated.*.md"], True),
    ("docs/notes.md", ["generated.*.md"], False),
    # A slash-less exact filename matches its basename at any depth too.
    ("a/b/.pre-commit-config.yaml", [".pre-commit-config.yaml"], True),
])
def test_is_excluded(path: str, patterns: list[str], expected: bool) -> None:
    assert is_excluded(path, patterns) is expected


def test_is_excluded_empty_patterns() -> None:
    assert is_excluded("anything.md", []) is False


# ---------- projects_root ----------

def test_projects_root_explicit_wins(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("PROJECTS_ROOT", "/should/be/ignored")
    assert projects_root(tmp_path) == tmp_path


def test_projects_root_env(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("PROJECTS_ROOT", str(tmp_path))
    assert projects_root() == tmp_path


def test_projects_root_default(monkeypatch) -> None:
    monkeypatch.delenv("PROJECTS_ROOT", raising=False)
    assert projects_root() == Path.home() / "projects"


# ---------- iter_workspace_repos ----------

def _make_repo(path: Path) -> None:
    (path / ".git").mkdir(parents=True)


def test_iter_workspace_repos_spans_org_dirs(tmp_path: Path) -> None:
    # ~/projects layout: org dirs holding repos, no .git on the org dirs.
    _make_repo(tmp_path / "coilysiren" / "repo-a")
    _make_repo(tmp_path / "coilyco-bridge" / "repo-b")
    _make_repo(tmp_path / "coilyco-flight-deck" / "repo-c")
    repos = iter_workspace_repos(tmp_path)
    # Sorted by (org dir, repo name): bridge, flight-deck, coilysiren.
    assert [r.name for r in repos] == ["repo-b", "repo-c", "repo-a"]
    assert (tmp_path / "coilyco-bridge" / "repo-b") in repos


def test_iter_workspace_repos_single_org_root(tmp_path: Path) -> None:
    # $PROJECTS_ROOT pointed straight at one org dir: children carry .git.
    _make_repo(tmp_path / "repo-a")
    _make_repo(tmp_path / "repo-b")
    repos = iter_workspace_repos(tmp_path)
    assert [r.name for r in repos] == ["repo-a", "repo-b"]


def test_iter_workspace_repos_skips_hidden(tmp_path: Path) -> None:
    _make_repo(tmp_path / "coilysiren" / "repo-a")
    # .dispatch-worktrees scaffolding under an org dir, and a hidden org dir.
    _make_repo(tmp_path / "coilysiren" / ".dispatch-worktrees" / "wt")
    _make_repo(tmp_path / ".hidden-org" / "repo-x")
    repos = iter_workspace_repos(tmp_path)
    assert [r.name for r in repos] == ["repo-a"]


def test_iter_workspace_repos_finds_org_profile_repos(tmp_path: Path) -> None:
    """Every org owns a repo literally named .github, and it is not scaffolding.

    The plain dotfile skip made all three invisible to every fleet rollout, so
    they never received the managed pre-commit block.
    """
    _make_repo(tmp_path / "coilysiren" / "repo-a")
    _make_repo(tmp_path / "coilysiren" / ".github")
    # Still scaffolding, and still skipped: only the profile repo is exempt.
    _make_repo(tmp_path / "coilysiren" / ".dispatch-worktrees" / "wt")
    repos = iter_workspace_repos(tmp_path)
    assert [r.name for r in repos] == [".github", "repo-a"]


def test_iter_workspace_repos_finds_org_profile_repo_in_single_org_root(
    tmp_path: Path,
) -> None:
    _make_repo(tmp_path / ".github")
    _make_repo(tmp_path / "repo-a")
    repos = iter_workspace_repos(tmp_path)
    assert [r.name for r in repos] == [".github", "repo-a"]


def test_iter_workspace_repos_skips_human_workdirs(tmp_path: Path) -> None:
    _make_repo(tmp_path / "coilysiren" / "repo-a")
    _make_repo(tmp_path / "coilysiren" / "repo-a-workdir")
    _make_repo(tmp_path / "coilysiren" / "repo-a-workdirs")
    repos = iter_workspace_repos(tmp_path)
    assert [r.name for r in repos] == ["repo-a", "repo-a-workdirs"]


def test_iter_workspace_repos_skips_human_workdirs_in_single_org_root(
    tmp_path: Path,
) -> None:
    _make_repo(tmp_path / "repo-a")
    _make_repo(tmp_path / "repo-a-workdir")
    repos = iter_workspace_repos(tmp_path)
    assert [r.name for r in repos] == ["repo-a"]


def test_iter_workspace_repos_skips_non_git_dirs(tmp_path: Path) -> None:
    _make_repo(tmp_path / "coilysiren" / "repo-a")
    (tmp_path / "coilysiren" / "not-a-repo").mkdir(parents=True)
    repos = iter_workspace_repos(tmp_path)
    assert [r.name for r in repos] == ["repo-a"]


# An empty fleet reads as a clean fleet in all seven callers, so a missing root
# has to refuse rather than return one (#7628).
def test_iter_workspace_repos_refuses_a_missing_root(tmp_path: Path) -> None:
    with pytest.raises(WorkspaceRootMissing):
        iter_workspace_repos(tmp_path / "nope")


def test_iter_workspace_repos_refuses_a_file_as_root(tmp_path: Path) -> None:
    root = tmp_path / "projects"
    root.write_text("not a directory")
    with pytest.raises(WorkspaceRootMissing):
        iter_workspace_repos(root)


# A real but empty workspace root is a different fact from an absent one, and
# stays an empty list rather than an error.
def test_iter_workspace_repos_allows_an_empty_root(tmp_path: Path) -> None:
    assert iter_workspace_repos(tmp_path) == []


def test_iter_workspace_repos_honors_env(monkeypatch, tmp_path: Path) -> None:
    _make_repo(tmp_path / "coilysiren" / "repo-a")
    monkeypatch.setenv("PROJECTS_ROOT", str(tmp_path))
    repos = iter_workspace_repos()
    assert [r.name for r in repos] == ["repo-a"]
