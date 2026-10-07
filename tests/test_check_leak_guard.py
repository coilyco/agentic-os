"""Tests for agentic_os.pre_commit.check_leak_guard: the encoded-leak guard.

Covers the three rule shapes (scope-all, repo-scoped, cycle-break) plus the
mechanics that make them safe: hex terms decoded only in memory, word-boundary
matching, only_globs / allow_globs, and that a violation never echoes the term.
The last section drives the real pre-commit binary, because which files the hook
sees is decided by pre-commit and not by the hook (COI-1743).
"""
from __future__ import annotations

import os
import shutil
import subprocess
import sys
from pathlib import Path

import yaml

from agentic_os.pre_commit import check_leak_guard as lg


def _hex(term: str) -> str:
    return term.encode("utf-8").hex()


def _rule(**over) -> dict:
    base = {"id": "r", "term_hex": _hex("needle"), "message": "fix it"}
    base.update(over)
    return base


# --- pure matching logic (no git, no fs) ------------------------------------

def test_compile_decodes_hex_and_word_boundary() -> None:
    matcher = lg._compile(_rule())
    assert matcher.search("a needle in here")
    assert not matcher.search("needles and threads")  # boundary: no match in-word


def test_compile_word_boundary_off_matches_substring() -> None:
    matcher = lg._compile(_rule(word_boundary=False))
    assert matcher.search("needles")


def test_compile_case_insensitive_by_default() -> None:
    assert lg._compile(_rule()).search("A NEEDLE")
    assert not lg._compile(_rule(case_sensitive=True)).search("A NEEDLE")


def test_compile_bad_hex_is_skipped_not_raised() -> None:
    assert lg._compile(_rule(term_hex="zzzz")) is None


def test_scan_reports_id_and_message_never_the_term() -> None:
    rule = _rule(id="employer", message="resolve at run time")
    hits = lg.scan("f.md", "the needle line\nclean line\n", rule, lg._compile(rule))
    assert hits == ["f.md:1: leak-guard[employer] - resolve at run time"]
    assert "needle" not in hits[0]  # the guard's own output is not a leak


def test_rule_applies_scope() -> None:
    assert lg._rule_applies(_rule(repos=None), "anything")
    assert lg._rule_applies(_rule(repos=["cli-guard"]), "cli-guard")
    assert not lg._rule_applies(_rule(repos=["cli-guard"]), "agentic-os")


# --- end-to-end through main() in a throwaway repo --------------------------

def _git(root: Path, *args: str) -> None:
    subprocess.run(["git", *args], cwd=root, check=True, capture_output=True)


def _repo(tmp_path: Path, remote: str = "agentic-os") -> Path:
    _git(tmp_path, "init")
    _git(tmp_path, "config", "user.email", "t@t")
    _git(tmp_path, "config", "user.name", "t")
    _git(tmp_path, "remote", "add", "origin",
         f"https://forgejo.coilysiren.me/coilyco-flight-deck/{remote}.git")
    return tmp_path


def _run(monkeypatch, tmp_path: Path, rules: list[dict]) -> int:
    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr(lg, "REPO_ROOT", tmp_path)
    monkeypatch.setattr(lg, "RULES", rules)
    return lg.main([])


def test_only_globs_enforces_front_page_alone(monkeypatch, tmp_path, capsys) -> None:
    repo = _repo(tmp_path)
    (repo / "README.md").write_text("names the needle\n")
    (repo / "tooling.py").write_text("needle is fine in tooling\n")
    _git(repo, "add", "-A")
    rule = _rule(repos=["agentic-os"], only_globs=["README.md"])
    assert _run(monkeypatch, repo, [rule]) == 1
    err = capsys.readouterr().err
    assert "README.md" in err and "tooling.py" not in err  # only the front page


def test_allow_globs_exempts_bio_surface(monkeypatch, tmp_path, capsys) -> None:
    repo = _repo(tmp_path)
    (repo / "resume.md").write_text("needle the employer\n")
    (repo / "config.toml").write_text("needle hardcoded\n")
    _git(repo, "add", "-A")
    rule = _rule(allow_globs=["resume.md"])
    assert _run(monkeypatch, repo, [rule]) == 1
    err = capsys.readouterr().err
    assert "config.toml" in err and "resume.md" not in err  # bio exempt


def test_out_of_scope_repo_passes(monkeypatch, tmp_path) -> None:
    repo = _repo(tmp_path, remote="agentic-os")
    (repo / "f.md").write_text("needle everywhere\n")
    _git(repo, "add", "-A")
    rule = _rule(repos=["cli-guard"])  # not this repo
    assert _run(monkeypatch, repo, [rule]) == 0


# --- the shipped tailnet-suffix rule (agentic-os#263) -----------------------

def _shipped(rule_id: str) -> dict:
    from agentic_os.pre_commit.leak_guard_rules import RULES

    return next(r for r in RULES if r["id"] == rule_id)


def test_tailnet_suffix_rule_fires_on_leaked_fqdn(monkeypatch, tmp_path, capsys) -> None:
    # The opaque suffix inside a full tailnet FQDN must trip the guard, the way
    # the openclaw baseUrl leaked it before this rule landed.
    rule = _shipped("tailnet-suffix-tower")
    suffix = bytes.fromhex(rule["term_hex"]).decode("utf-8")
    repo = _repo(tmp_path)
    (repo / "openclaw.json").write_text(
        f'{{"baseUrl": "http://kai-tower-3026.{suffix}.ts.net:11434"}}\n'
    )
    _git(repo, "add", "-A")
    assert _run(monkeypatch, repo, [rule]) == 1
    err = capsys.readouterr().err
    assert "openclaw.json" in err
    assert suffix not in err  # the guard's own output must not re-leak the term


def test_tailnet_suffix_rule_passes_on_env_placeholder(monkeypatch, tmp_path) -> None:
    # The de-leaked form: a run-time env var, no opaque suffix in tracked config.
    rule = _shipped("tailnet-suffix-tower")
    repo = _repo(tmp_path)
    (repo / "openclaw.json").write_text('{"baseUrl": "${OLLAMA_BASE_URL}"}\n')
    _git(repo, "add", "-A")
    assert _run(monkeypatch, repo, [rule]) == 0


def test_umbra_ward_cycle_fires_on_the_module_edge(monkeypatch, tmp_path, capsys) -> None:
    # The cycle this rule exists to stop: a require line naming the upper layer.
    rule = _shipped("umbra-ward-cycle")
    term = bytes.fromhex(rule["term_hex"]).decode("utf-8")
    repo = _repo(tmp_path, remote="umbra")
    (repo / "go.mod").write_text(f"module umbra\n\nrequire forgejo/{term} v1.0.0\n")
    _git(repo, "add", "-A")
    assert _run(monkeypatch, repo, [rule]) == 1
    assert "go.mod" in capsys.readouterr().err


def test_umbra_ward_cycle_ignores_prose_naming_the_consumer(monkeypatch, tmp_path) -> None:
    # umbra names ".ward" as the example consumer in the very files that document
    # hardcoding none; unscoped this rule flagged 375 such lines and blocked the repo.
    rule = _shipped("umbra-ward-cycle")
    term = bytes.fromhex(rule["term_hex"]).decode("utf-8")
    repo = _repo(tmp_path, remote="umbra")
    (repo / "appdir.go").write_text(f'// The consumer dir, e.g. ".{term}".\npackage config\n')
    _git(repo, "add", "-A")
    assert _run(monkeypatch, repo, [rule]) == 0


# --- which files the hook scans, through the real pre-commit (COI-1743) ------

REPO_ROOT = Path(__file__).resolve().parent.parent

# Stands in for the hook entry so the run uses a fake rule and never a shipped term.
_SHIM = """\
import sys
from agentic_os.pre_commit import check_leak_guard as lg

lg.RULES = [{{"id": "fake", "term_hex": "{term}", "message": "fix it"}}]
sys.exit(lg.main())
"""


def _wire(repo: Path, monkeypatch) -> None:
    """Point pre-commit in `repo` at the shipped leak-guard definition, fake rule."""
    hooks = yaml.safe_load((REPO_ROOT / ".pre-commit-hooks.yaml").read_text(encoding="utf-8"))
    shipped = next(h for h in hooks if h["id"] == "leak-guard")
    shim = repo.parent / "shim.py"
    shim.write_text(_SHIM.format(term=_hex("needle")), encoding="utf-8")
    # Only the entry and language change. The file selection fields come from the
    # shipped definition, which is the thing under test.
    local = {k: shipped[k] for k in ("id", "name", "always_run", "pass_filenames", "stages")}
    local |= {"language": "system", "entry": f'"{sys.executable}" "{shim}"'}
    config = {"repos": [{"repo": "local", "hooks": [local]}]}
    (repo / ".pre-commit-config.yaml").write_text(yaml.safe_dump(config), encoding="utf-8")
    for key in [k for k in os.environ if k.startswith("GIT_")]:
        monkeypatch.delenv(key)  # pre-commit exports GIT_INDEX_FILE and friends
    monkeypatch.setenv("PYTHONPATH", str(REPO_ROOT))
    monkeypatch.setenv("PRE_COMMIT_HOME", str(repo.parent / "pc-home"))
    monkeypatch.chdir(repo)


def _sh(*cmd: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(cmd, text=True, capture_output=True, check=False)


def _pre_commit(*args: str) -> subprocess.CompletedProcess[str]:
    # Absent binary is an environment defect, not a reason to skip: CI runs it too.
    binary = shutil.which("pre-commit")
    assert binary, "pre-commit must be on PATH for the leak-guard file-selection tests"
    return _sh(binary, *args)


def _leaky_repo(tmp_path: Path, monkeypatch) -> Path:
    """A repo whose committed tree leaks, with an unrelated clean file ready to stage."""
    (tmp_path / "repo").mkdir()
    repo = _repo(tmp_path / "repo")
    _wire(repo, monkeypatch)
    (repo / "leaky.md").write_text("names the needle\n")
    (repo / "clean.txt").write_text("fine\n")
    _git(repo, "add", "-A")
    _git(repo, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "base", "--no-verify")
    return repo


def _violations(result: subprocess.CompletedProcess[str]) -> list[str]:
    return [ln for ln in result.stdout.splitlines() if ln.startswith("FAIL:")]


def test_all_files_ignores_what_is_staged(monkeypatch, tmp_path) -> None:
    repo = _leaky_repo(tmp_path, monkeypatch)
    empty_index = _pre_commit("run", "leak-guard", "--all-files")
    (repo / "clean.txt").write_text("fine, edited\n")
    _git(repo, "add", "clean.txt")
    one_staged = _pre_commit("run", "leak-guard", "--all-files")
    assert empty_index.returncode == 1, empty_index.stdout
    assert _violations(empty_index) == ["FAIL: leaky.md:1: leak-guard[fake] - fix it"]
    assert one_staged.returncode == empty_index.returncode
    assert _violations(one_staged) == _violations(empty_index)


def test_commit_stage_scans_only_the_staged_files(monkeypatch, tmp_path) -> None:
    repo = _leaky_repo(tmp_path, monkeypatch)
    (repo / "clean.txt").write_text("fine, edited\n")
    _git(repo, "add", "clean.txt")
    # leaky.md is tracked and leaks but is not part of this commit.
    assert _pre_commit("run", "leak-guard").returncode == 0


def test_commit_stage_scans_staged_content_not_the_working_tree(monkeypatch, tmp_path) -> None:
    repo = _leaky_repo(tmp_path, monkeypatch)
    _pre_commit("install")
    (repo / "clean.txt").write_text("needle, staged\n")
    _git(repo, "add", "clean.txt")
    (repo / "clean.txt").write_text("fixed after staging\n")  # unstaged, so not committed
    result = _sh("git", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "x")
    assert result.returncode == 1, result.stdout + result.stderr
    assert "FAIL: clean.txt:1: leak-guard[fake]" in result.stdout + result.stderr


def test_explicit_paths_are_read_from_the_working_tree(monkeypatch, tmp_path) -> None:
    repo = _repo(tmp_path)
    (repo / "clean.txt").write_text("fine\n")
    (repo / "leaky.md").write_text("names the needle\n")
    _git(repo, "add", "clean.txt")  # staged and unrelated, as in COI-1743
    monkeypatch.chdir(repo)
    monkeypatch.setattr(lg, "REPO_ROOT", repo)
    monkeypatch.setattr(lg, "RULES", [_rule()])
    assert lg.main(["leaky.md"]) == 1
    assert lg.main(["clean.txt"]) == 0
