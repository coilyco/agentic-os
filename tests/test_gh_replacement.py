"""Static and behavioral contract for the guarded `gh` replacement binary."""

from __future__ import annotations

import os
import re
import shutil
import subprocess
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parents[1]
PROJECT = ROOT / ".umbra" / "shims"
MEMBER = PROJECT / "gh" / "gh.kdl"
LOCK = ROOT / ".umbra" / "guardfiles" / "specverb.lock"

# The pull-request write withheld for every repo. The rest are granted per
# GitHub-canonical repo instead (COI-2975).
WITHHELD = ("review",)

# Named by no list, so a refusal here is the allowlist and not a typo'd repo.
UNLISTED = "coilyco/not-a-listed-repo"


@pytest.fixture(scope="module")
def shim(tmp_path_factory: pytest.TempPathFactory) -> Path:
    suffix = ".exe" if os.name == "nt" else ""
    binary = tmp_path_factory.mktemp("gh-shim") / f"gh{suffix}"
    # umbra reads the lock from the project root and rejects a symlink out of
    # it, so this stages the copy `just gh-shim-build` stages.
    shutil.copyfile(LOCK, PROJECT / "specverb.lock")
    subprocess.run(
        [
            "umbra",
            "--project-root",
            str(PROJECT),
            "--guardfile",
            str(MEMBER),
            "build",
            "--out",
            str(binary),
        ],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    return binary


@pytest.fixture(scope="module")
def fake_gh(tmp_path_factory: pytest.TempPathFactory) -> Path:
    """A stand-in for the real gh, so a granted verb is observable offline."""
    bin_dir = tmp_path_factory.mktemp("real-gh")
    script = bin_dir / "gh"
    script.write_text('#!/bin/sh\nprintf "REAL_GH %s\\n" "$*"\n', encoding="utf-8")
    script.chmod(0o755)
    return bin_dir


def run(shim: Path, fake_gh: Path, *args: str) -> subprocess.CompletedProcess[str]:
    env = os.environ.copy()
    # The shim's own directory is excluded from resolution by umbra, and the
    # fake has to be findable, so PATH is fake-first with nothing else needed.
    env["PATH"] = f"{fake_gh}{os.pathsep}{env['PATH']}"
    return subprocess.run(
        [str(shim), *args], capture_output=True, text=True, env=env, check=False
    )


@pytest.mark.parametrize("verb", WITHHELD)
def test_every_pull_request_write_is_withheld_and_says_where_it_went(
    shim: Path, fake_gh: Path, verb: str
) -> None:
    result = run(shim, fake_gh, "pr", verb, "12")

    assert result.returncode == 2, result.stdout + result.stderr
    combined = result.stdout + result.stderr
    assert "withheld" in combined
    assert "Forgejo" in combined or "forgejo" in combined
    assert "REAL_GH" not in combined, "a withheld leaf must reach no binary"


@pytest.mark.parametrize("verb", WITHHELD)
def test_the_withheld_leaves_are_visible_in_help(
    shim: Path, fake_gh: Path, verb: str
) -> None:
    """Absence cannot say whether a verb is refused or missing. This is the point."""
    result = run(shim, fake_gh, "pr", "--help")

    assert verb in result.stdout
    assert "NOT AVAILABLE" in result.stdout


def test_a_read_reaches_the_real_binary(shim: Path, fake_gh: Path) -> None:
    result = run(shim, fake_gh, "pr", "list", "--state", "open")

    assert result.returncode == 0, result.stdout + result.stderr
    assert "REAL_GH pr list --state open" in result.stdout


def test_viewing_and_checks_reach_the_real_binary_for_any_repo(
    shim: Path, fake_gh: Path
) -> None:
    for args in (
        ("pr", "view", "12", "--repo", UNLISTED),
        ("pr", "checks", "12", "--repo", UNLISTED),
    ):
        result = run(shim, fake_gh, *args)
        assert result.returncode == 0, (args, result.stdout + result.stderr)
        assert f"REAL_GH {' '.join(args)}" in result.stdout, args


def test_an_unnamed_verb_forwards_to_the_real_binary(
    shim: Path, fake_gh: Path
) -> None:
    """Under default-allow this file is a boundary rather than a catalog."""
    result = run(shim, fake_gh, "workflow", "run", "x")

    assert result.returncode == 0, result.stdout + result.stderr
    assert "REAL_GH workflow run x" in result.stdout


def test_an_unnamed_sibling_of_a_withheld_verb_keeps_its_flags(
    shim: Path, fake_gh: Path
) -> None:
    """A withheld `pr create` mounts a `pr` group, and a group parses flags.

    Without umbra forwarding the group's usage errors, `--json` would die here
    purely because a sibling is withheld.
    """
    result = run(shim, fake_gh, "pr", "view", "12", "--json", "title")

    assert result.returncode == 0, result.stdout + result.stderr
    assert "REAL_GH pr view 12 --json title" in result.stdout


def test_the_api_leaf_admits_no_flag_that_could_write(
    shim: Path, fake_gh: Path
) -> None:
    """gh api flips to POST on a field flag alone, so only an allowlist sees it."""
    for args in (
        ("api", "--method", "POST", "/repos/x/y/pulls"),
        ("api", "-X", "POST", "/repos/x/y/pulls"),
        ("api", "-f", "title=x", "/repos/x/y/pulls"),
        ("api", "--input", "-", "/repos/x/y/pulls"),
    ):
        result = run(shim, fake_gh, *args)
        assert result.returncode != 0, args
        assert "REAL_GH" not in result.stdout, args

    allowed = run(shim, fake_gh, "api", "repos/x/y", "--cache", "1h")
    assert allowed.returncode == 0, allowed.stdout + allowed.stderr
    assert "REAL_GH api repos/x/y --cache 1h" in allowed.stdout


def refused(result: subprocess.CompletedProcess[str]) -> bool:
    return result.returncode == 2 and "REAL_GH" not in result.stdout


@pytest.fixture(scope="module")
def listed(shim: Path, fake_gh: Path) -> list[str]:
    """The GitHub-canonical repos, as the guard itself reports them on a refusal.

    Read from umbra's own refusal text rather than the KDL, so the list has one
    owner and these tests never restate it.
    """
    result = run(shim, fake_gh, "pr", "create", "--repo", UNLISTED, "--title", "x")
    found = re.search(r"allowed pattern \[([^\]]*)\]", result.stderr)
    assert found and found.group(1).split(), result.stderr
    return found.group(1).split()


def test_a_listed_repo_accepts_every_granted_pull_request_verb(
    shim: Path, fake_gh: Path, listed: list[str]
) -> None:
    for repo in listed:
        for args in (
            ("pr", "create", "--repo", repo, "--title", "t", "--body", "see https://x.test/1"),
            ("pr", "create", f"--repo={repo}", "-t", "t", "-b", "b"),
            ("pr", "edit", "12", "--repo", repo, "--title", "t", "--add-label", "bug"),
            ("pr", "merge", "12", "--repo", repo, "--squash", "--delete-branch"),
            ("pr", "merge", "--squash", "12", f"--repo={repo}"),
            ("pr", "close", "12", "--repo", repo, "--comment", "done"),
            ("pr", "reopen", "12", "--repo", repo),
            ("pr", "comment", "12", "--repo", repo, "--body", "see https://x.test/1"),
            ("pr", "ready", "12", "--repo", repo, "--undo"),
        ):
            result = run(shim, fake_gh, *args)
            assert result.returncode == 0, (args, result.stdout + result.stderr)
            assert f"REAL_GH {' '.join(args)}" in result.stdout, args


def test_every_list_names_the_same_repos(
    shim: Path, fake_gh: Path, listed: list[str]
) -> None:
    """One repo joins every list in the same change, or it is half flipped."""
    for verb in ("edit", "merge", "close", "reopen", "comment", "ready"):
        result = run(shim, fake_gh, "pr", verb, "12", "--repo", UNLISTED)
        assert f"[{' '.join(listed)}]" in result.stderr, (verb, result.stderr)
    result = run(shim, fake_gh, "actions-permissions", "repos/x/y/actions/permissions")
    endpoints = [f"repos/{repo}/actions/permissions" for repo in listed]
    assert f"[{' '.join(endpoints)}]" in result.stderr, result.stderr


def test_an_unlisted_repo_is_refused_on_every_granted_verb(
    shim: Path, fake_gh: Path
) -> None:
    for args in (
        ("pr", "create", "--repo", UNLISTED, "--title", "t"),
        ("pr", "edit", "12", "--repo", UNLISTED, "--title", "t"),
        ("pr", "merge", "12", "--repo", UNLISTED, "--squash"),
        ("pr", "close", "12", "--repo", UNLISTED),
        ("pr", "reopen", "12", "--repo", UNLISTED),
        ("pr", "comment", "12", "--repo", UNLISTED, "--body", "t"),
        ("pr", "ready", "12", "--repo", UNLISTED),
        ("actions-permissions", f"repos/{UNLISTED}/actions/permissions", "--field", "enabled=true"),
    ):
        assert refused(run(shim, fake_gh, *args)), args


def test_a_call_that_names_no_repo_is_refused(shim: Path, fake_gh: Path) -> None:
    """cwd inference is invisible to the guard, so --repo is required."""
    for args in (
        ("pr", "create", "--title", "t"),
        ("pr", "edit", "12", "--title", "t"),
        ("pr", "merge", "12", "--squash"),
        ("pr", "close", "12"),
        ("pr", "reopen", "12"),
        ("pr", "comment", "12", "--body", "t"),
        ("pr", "ready", "12"),
    ):
        assert refused(run(shim, fake_gh, *args)), args


def test_the_short_repo_flag_cannot_retarget_a_listed_repo(
    shim: Path, fake_gh: Path, listed: list[str]
) -> None:
    """gh binds -R and --repo to one variable and the last one wins."""
    repo = listed[0]
    for verb_args in (
        ("create", "--title", "t"),
        ("edit", "12"),
        ("merge", "12", "--squash"),
        ("close", "12"),
        ("reopen", "12"),
        ("comment", "12", "--body", "t"),
        ("ready", "12"),
    ):
        for flag in (("-R", UNLISTED), (f"-R{UNLISTED}",)):
            args = ("pr", verb_args[0], *verb_args[1:], "--repo", repo, *flag)
            assert refused(run(shim, fake_gh, *args)), args


def test_a_url_selector_cannot_retarget_a_listed_repo(
    shim: Path, fake_gh: Path, listed: list[str]
) -> None:
    """gh lets a pull request URL override --repo, in either flag order."""
    url = f"https://github.com/{UNLISTED}/pull/3"
    repo = listed[0]
    for verb, extra in (
        ("merge", ("--squash",)),
        ("edit", ("--title", "t")),
        ("close", ()),
        ("reopen", ()),
        ("comment", ("--body", "t")),
        ("ready", ()),
    ):
        for args in (
            ("pr", verb, url, "--repo", repo, *extra),
            ("pr", verb, *extra, url, "--repo", repo),
        ):
            assert refused(run(shim, fake_gh, *args)), args


def test_merge_is_squash_only(shim: Path, fake_gh: Path, listed: list[str]) -> None:
    repo = listed[0]
    for flag in ("--merge", "-m", "--rebase", "-r", "--admin"):
        assert refused(run(shim, fake_gh, "pr", "merge", "12", "--repo", repo, flag)), flag
    assert run(shim, fake_gh, "pr", "merge", "12", "--repo", repo, "-s").returncode == 0


def test_actions_permissions_is_a_put_on_one_endpoint(
    shim: Path, fake_gh: Path, listed: list[str]
) -> None:
    """The write surface is gh api --method PUT, and nothing else can be spelled."""
    endpoint = f"repos/{listed[0]}/actions/permissions"
    ok = run(shim, fake_gh, "actions-permissions", endpoint, "--field", "enabled=true")
    assert ok.returncode == 0, ok.stdout + ok.stderr
    assert f"REAL_GH api --method PUT {endpoint} --field enabled=true" in ok.stdout

    for args in (
        ("actions-permissions", endpoint, "--method", "DELETE"),
        ("actions-permissions", endpoint, "-F", "enabled=true"),
        ("actions-permissions", f"{endpoint}/../../../{UNLISTED}"),
        ("actions-permissions", f"repos/{listed[0]}/actions/secrets"),
    ):
        assert refused(run(shim, fake_gh, *args)), args


@pytest.mark.xfail(
    strict=True,
    reason="umbra reads the first --repo and gh the last, so a repeat retargets (COI-2977)",
)
def test_a_repeated_repo_flag_is_refused(
    shim: Path, fake_gh: Path, listed: list[str]
) -> None:
    result = run(shim, fake_gh, "pr", "create", "--repo", listed[0], "--repo", UNLISTED, "--title", "t")
    assert refused(result)


def test_the_guardfile_grants_no_pull_request_write(shim: Path) -> None:
    """A static read, so a `can run` added beside a withhold fails here first."""
    source = MEMBER.read_text(encoding="utf-8")
    for verb in WITHHELD:
        assert f'can run "pr {verb}"' not in source
        assert f"withhold pr {verb} {{" in source


def test_the_guardfile_declares_default_allow_with_its_reason(shim: Path) -> None:
    """Losing the declaration would silently close the tool back down."""
    source = MEMBER.read_text(encoding="utf-8")

    assert "default-allow {" in source
    assert "reason" in source.split("default-allow {", 1)[1][:600]


def test_the_replacement_declares_itself(shim: Path, fake_gh: Path) -> None:
    """UMBRA_IDENTIFY is the only surface that says what stands on the name."""
    env = os.environ.copy()
    env["PATH"] = f"{fake_gh}{os.pathsep}{env['PATH']}"
    env["UMBRA_IDENTIFY"] = "1"
    result = subprocess.run(
        [str(shim)], capture_output=True, text=True, env=env, check=False
    )

    assert "umbra replacement" in result.stdout + result.stderr
