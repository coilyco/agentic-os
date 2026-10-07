"""Tests for the staged text hygiene guards.

The last section drives the real pre-commit binary, because which files a hook sees
is decided by pre-commit and not by the hook (COI-2460, the same defect as COI-1743).
"""
from __future__ import annotations

import os
import shutil
import subprocess
import sys
from pathlib import Path

import pytest
import yaml

try:
    import tomllib
except ModuleNotFoundError:  # pragma: no cover - Python 3.10
    import tomli as tomllib

import agentic_os.config as cfg
from agentic_os.pre_commit import text_scan
from agentic_os.pre_commit import check_issue_references as ir
from agentic_os.pre_commit import check_unresolved_placeholders as up

REPO_ROOT = Path(__file__).resolve().parent.parent


def _project_scripts() -> dict[str, str]:
    data = tomllib.loads((REPO_ROOT / "pyproject.toml").read_text(encoding="utf-8"))
    scripts = data["project"]["scripts"]
    return {str(name): str(entry) for name, entry in scripts.items()}


def _published_python_hooks() -> dict[str, str]:
    hooks = yaml.safe_load(
        (REPO_ROOT / ".pre-commit-hooks.yaml").read_text(encoding="utf-8")
    )
    return {
        str(hook["id"]): str(hook["entry"])
        for hook in hooks
        if hook.get("language") == "python"
    }


def _git(root: Path, *args: str) -> None:
    subprocess.run(["git", *args], cwd=root, check=True, capture_output=True)


def _repo(tmp_path: Path) -> Path:
    _git(tmp_path, "init")
    _git(tmp_path, "config", "user.email", "t@t")
    _git(tmp_path, "config", "user.name", "t")
    return tmp_path


def _write(repo: Path, rel: str, text: str) -> None:
    path = repo / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


def test_placeholder_guard_flags_unfinished_prose(monkeypatch, tmp_path: Path, capsys) -> None:
    repo = _repo(tmp_path)
    _write(repo, "pyproject.toml", """
[tool.agentic-os.unresolved-placeholder-guard]
enabled = true
""")
    _write(repo, "docs/todo.md", "TODO implement the rest\n")
    _git(repo, "add", "-A")
    monkeypatch.chdir(repo)
    monkeypatch.setattr(cfg, "REPO_ROOT", repo, raising=True)
    monkeypatch.setattr(text_scan, "REPO_ROOT", repo, raising=True)
    assert up.main([]) == 1
    err = capsys.readouterr().err
    assert "todo-implement" in err


def test_placeholder_guard_honors_allowlist(monkeypatch, tmp_path: Path) -> None:
    repo = _repo(tmp_path)
    _write(repo, "pyproject.toml", """
[tool.agentic-os.unresolved-placeholder-guard]
enabled = true
allow_globs = ["docs/examples/**"]
""")
    _write(repo, "docs/examples/example.md", "placeholder text\n")
    _git(repo, "add", "-A")
    monkeypatch.chdir(repo)
    monkeypatch.setattr(cfg, "REPO_ROOT", repo, raising=True)
    monkeypatch.setattr(text_scan, "REPO_ROOT", repo, raising=True)
    assert up.main([]) == 0


def test_issue_guard_flags_direct_refs(monkeypatch, tmp_path: Path, capsys) -> None:
    repo = _repo(tmp_path)
    _write(repo, "pyproject.toml", """
[tool.agentic-os.issue-reference-guard]
enabled = true
""")
    _write(repo, "README.md", "See #337 for the draft\n")
    _git(repo, "add", "-A")
    monkeypatch.chdir(repo)
    monkeypatch.setattr(cfg, "REPO_ROOT", repo, raising=True)
    monkeypatch.setattr(text_scan, "REPO_ROOT", repo, raising=True)
    assert ir.main([]) == 1
    err = capsys.readouterr().err
    assert "bare-issue-ref" in err


def test_issue_guard_honors_allowlist(monkeypatch, tmp_path: Path) -> None:
    repo = _repo(tmp_path)
    _write(repo, "pyproject.toml", """
[tool.agentic-os.issue-reference-guard]
enabled = true
allow_globs = ["docs/examples/**"]
""")
    _write(repo, "docs/examples/example.md", "See #337 for the draft\n")
    _git(repo, "add", "-A")
    monkeypatch.chdir(repo)
    monkeypatch.setattr(cfg, "REPO_ROOT", repo, raising=True)
    monkeypatch.setattr(text_scan, "REPO_ROOT", repo, raising=True)
    assert ir.main([]) == 0


def test_issue_guard_ignores_code_examples_test_fixtures_and_upstream_links(
    monkeypatch, tmp_path: Path, capsys
) -> None:
    repo = _repo(tmp_path)
    _write(repo, "pyproject.toml", """
[tool.agentic-os.issue-reference-guard]
enabled = true
""")
    _write(
        repo,
        "docs/examples.md",
        """# Examples

```bash
gh pr create --body "Closes #42"
```

`owner/repo#88`

> ward agent claude work owner/repo#88 --new-tab

See https://warpdotdev/Warp/issues/2579 for the upstream workaround.
""",
    )
    _write(repo, "tests/fixture.md", "See #999 in the fixture\n")
    _write(repo, "docs/prose.md", "See #337 for the draft\n")
    _git(repo, "add", "-A")
    monkeypatch.chdir(repo)
    monkeypatch.setattr(cfg, "REPO_ROOT", repo, raising=True)
    monkeypatch.setattr(text_scan, "REPO_ROOT", repo, raising=True)
    assert ir.main([]) == 1
    err = capsys.readouterr().err
    assert err.count("FAIL:") == 1
    assert "bare-issue-ref" in err
    assert "scoped-issue-ref" not in err
    assert "issue-url" not in err


def test_published_python_hook_entries_have_console_scripts() -> None:
    scripts = _project_scripts()
    hooks = _published_python_hooks()
    missing = {
        hook_id: entry
        for hook_id, entry in hooks.items()
        if entry not in scripts
    }
    assert not missing, (
        "published hook entries must be exported from [project.scripts]: "
        f"{missing}"
    )


# --- which files each guard scans, through the real pre-commit (COI-2460) ----

# hook id -> (entry module, a line it rejects, the rule id that rejects it)
_GUARDS = {
    "issue-reference-guard": (
        "agentic_os.pre_commit.check_issue_references",
        "See #337 for the draft\n",
        "bare-issue-ref",
    ),
    "unresolved-placeholder-guard": (
        "agentic_os.pre_commit.check_unresolved_placeholders",
        "TODO implement the rest\n",
        "todo-implement",
    ),
}


def _wire(repo: Path, hook_id: str, module: str, monkeypatch) -> None:
    """Point pre-commit in `repo` at the shipped definition of `hook_id`."""
    hooks = yaml.safe_load((REPO_ROOT / ".pre-commit-hooks.yaml").read_text(encoding="utf-8"))
    shipped = next(h for h in hooks if h["id"] == hook_id)
    # Only the entry and language change. The file selection fields come from the
    # shipped definition, which is the thing under test.
    local = {k: shipped[k] for k in ("id", "name", "always_run", "pass_filenames", "stages")}
    local |= {"language": "system", "entry": f'"{sys.executable}" -m {module}'}
    config = {"repos": [{"repo": "local", "hooks": [local]}]}
    (repo / ".pre-commit-config.yaml").write_text(yaml.safe_dump(config), encoding="utf-8")
    # The config names the hook id, which the placeholder guard would itself flag.
    _write(repo, "pyproject.toml", f"""
[tool.agentic-os.{hook_id}]
enabled = true
excludes = [".pre-commit-config.yaml"]
""")
    for key in [k for k in os.environ if k.startswith("GIT_")]:
        monkeypatch.delenv(key)  # pre-commit exports GIT_INDEX_FILE and friends
    monkeypatch.setenv("PYTHONPATH", str(REPO_ROOT))
    monkeypatch.setenv("PRE_COMMIT_HOME", str(repo.parent / "pc-home"))
    monkeypatch.chdir(repo)


def _pre_commit(hook_id: str, *args: str) -> subprocess.CompletedProcess[str]:
    # Absent binary is an environment defect, not a reason to skip: CI runs it too.
    binary = shutil.which("pre-commit")
    assert binary, "pre-commit must be on PATH for the text guard file-selection tests"
    cmd = [binary, "run", hook_id, "--hook-stage", "manual", *args]
    return subprocess.run(cmd, text=True, capture_output=True, check=False)


def _violations(result: subprocess.CompletedProcess[str]) -> list[str]:
    return [ln for ln in result.stdout.splitlines() if ln.startswith("FAIL:")]


def _dirty_repo(tmp_path: Path, hook_id: str, monkeypatch) -> Path:
    """A repo whose committed tree violates, with an unrelated clean file ready to stage."""
    module, bad, _rule = _GUARDS[hook_id]
    (tmp_path / "repo").mkdir()
    repo = _repo(tmp_path / "repo")
    _wire(repo, hook_id, module, monkeypatch)
    _write(repo, "dirty.md", bad)
    _write(repo, "clean.txt", "fine\n")
    _git(repo, "add", "-A")
    _git(repo, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "base", "--no-verify")
    return repo


@pytest.mark.parametrize("hook_id", sorted(_GUARDS))
def test_all_files_ignores_what_is_staged(hook_id, monkeypatch, tmp_path: Path) -> None:
    repo = _dirty_repo(tmp_path, hook_id, monkeypatch)
    rule = _GUARDS[hook_id][2]
    empty_index = _pre_commit(hook_id, "--all-files")
    _write(repo, "clean.txt", "fine, edited\n")
    _git(repo, "add", "clean.txt")
    one_staged = _pre_commit(hook_id, "--all-files")
    assert empty_index.returncode == 1, empty_index.stdout
    assert [v.split(" - ")[0] for v in _violations(empty_index)] == [f"FAIL: dirty.md:1: {rule}"]
    assert one_staged.returncode == empty_index.returncode
    assert _violations(one_staged) == _violations(empty_index)


@pytest.mark.parametrize("hook_id", sorted(_GUARDS))
def test_without_all_files_scans_only_the_staged_files(hook_id, monkeypatch, tmp_path: Path) -> None:
    repo = _dirty_repo(tmp_path, hook_id, monkeypatch)
    _write(repo, "clean.txt", "fine, edited\n")
    _git(repo, "add", "clean.txt")
    # dirty.md is tracked and violates but is not part of this run.
    assert _pre_commit(hook_id).returncode == 0
