"""Tests for the pre-push PR guard.

A branch whose PR already merged is still pushable on three of the four repos,
so the push succeeds, nothing points at the branch, and the work is stranded.
GitHub is the only source that sees a squash or rebase merge, whose branch tip is
not an ancestor of the commit it merged as. See agentic-os#1034.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
GUARD = ROOT / "scripts" / "pr-guard-pre-push.sh"

BRANCH = "aos/feature"
REMOTE_URL = "https://github.com/coilyco/agentic-os.git"


def _stub_gh(
    bin_dir: Path,
    *,
    open_prs: list,
    closed_prs: list,
    published: bool = True,
    authenticated: bool = True,
    lookup_fails: bool = False,
) -> Path:
    """A `gh` that answers like GitHub does after `--head` filtered the listing.

    It reports the argv it received to `argv.log`, one call per line, so a test
    can see which repo and state the guard asked about.
    """
    def on_branch(prs: list) -> list:
        return [p for p in prs if p["head"]["ref"] == BRANCH]

    open_count = len(on_branch(open_prs))
    merged = [p["number"] for p in on_branch(closed_prs) if p["merged_at"]]
    merged_answer = str(max(merged)) if merged else ""
    gh = bin_dir / "gh"
    gh.write_text(
        f"""#!/usr/bin/env bash
printf '%s\\n' "$*" >> "{bin_dir}/argv.log"
case "$1 $2" in
  "auth status") exit {0 if authenticated else 1} ;;
  "api repos/"*) exit {0 if published else 1} ;;
esac
[ "$1 $2" = "pr list" ] || exit 1
[ "{int(lookup_fails)}" = 1 ] && exit 1
case "$*" in
  *"--state open"*) printf '%s\\n' {open_count} ;;
  *"--state merged"*) printf '%s\\n' '{merged_answer}' ;;
  *) exit 1 ;;
esac
""",
        encoding="utf-8",
    )
    gh.chmod(0o755)
    return gh


def _run(
    tmp_path: Path,
    *,
    open_prs: list,
    closed_prs: list,
    published: bool = True,
    authenticated: bool = True,
    lookup_fails: bool = False,
    branch: str = BRANCH,
    lane: str | None = None,
    remote_url: str = REMOTE_URL,
    with_gh: bool = True,
):
    """Run the guard against a `gh` stub serving fixed PR listings.

    `lane` installs a `generate-git-workflow` stub on PATH answering
    `--print-lane` with that slug, which is the resolution path a consumer
    takes. `None` installs no stub, so the guard finds no reader.
    """
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    if lane is not None:
        reader = bin_dir / "generate-git-workflow"
        reader.write_text(
            f"#!/usr/bin/env bash\nprintf '{lane}\\n'\n", encoding="utf-8"
        )
        reader.chmod(0o755)
    if with_gh:
        _stub_gh(
            bin_dir,
            open_prs=open_prs,
            closed_prs=closed_prs,
            published=published,
            authenticated=authenticated,
            lookup_fails=lookup_fails,
        )

    # A real checkout, so the default-branch lookup behaves as it does in anger.
    repo = tmp_path / "repo"
    repo.mkdir()
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)

    return subprocess.run(
        ["bash", str(GUARD)],
        cwd=repo,
        check=False,
        capture_output=True,
        text=True,
        env=os.environ
        | {
            "PATH": f"{bin_dir}{os.pathsep}{os.environ['PATH']}",
            "PRE_COMMIT_REMOTE_NAME": "origin",
            "PRE_COMMIT_REMOTE_URL": remote_url,
            "PRE_COMMIT_REMOTE_BRANCH": f"refs/heads/{branch}",
        },
    )


def _pr(number: int, *, merged: bool, state: str) -> dict:
    return {
        "number": number,
        "state": state,
        "head": {"ref": BRANCH},
        "merged_at": "2026-08-01T00:00:00Z" if merged else None,
    }


def test_a_merged_branch_is_refused_and_names_its_pr(tmp_path: Path) -> None:
    got = _run(tmp_path, open_prs=[], closed_prs=[_pr(1030, merged=True, state="closed")])

    assert got.returncode == 1
    assert "already merged as PR #1030" in got.stderr
    assert "strand the" in got.stderr


def test_an_open_pr_still_allows_the_push(tmp_path: Path) -> None:
    got = _run(
        tmp_path,
        open_prs=[_pr(1191, merged=False, state="open")],
        closed_prs=[_pr(1030, merged=True, state="closed")],
    )

    assert got.returncode == 0, got.stderr


def test_a_closed_unmerged_pr_is_not_a_merged_pr(tmp_path: Path) -> None:
    # A PR closed without merging leaves the branch alive, so the advice is
    # "open one", not "branch again".
    got = _run(tmp_path, open_prs=[], closed_prs=[_pr(1030, merged=False, state="closed")])

    assert got.returncode == 1
    assert "has no open PR" in got.stderr
    assert "already merged" not in got.stderr
    assert "https://github.com/coilyco/agentic-os/compare/main...aos/feature" in got.stderr


def test_the_newest_merged_pr_is_the_one_named(tmp_path: Path) -> None:
    got = _run(
        tmp_path,
        open_prs=[],
        closed_prs=[
            _pr(900, merged=True, state="closed"),
            _pr(1030, merged=True, state="closed"),
        ],
    )

    assert "already merged as PR #1030" in got.stderr


def test_a_different_branch_merged_pr_is_ignored(tmp_path: Path) -> None:
    other = _pr(1030, merged=True, state="closed")
    other["head"] = {"ref": "aos/somebody-else"}
    got = _run(tmp_path, open_prs=[], closed_prs=[other])

    assert "already merged" not in got.stderr


def test_an_unpublished_branch_is_left_alone(tmp_path: Path) -> None:
    # The first push of a branch cannot have a PR: it creates the ref one needs.
    got = _run(tmp_path, open_prs=[], closed_prs=[], published=False)

    assert got.returncode == 0, got.stderr


def test_an_unauthenticated_gh_never_blocks_a_push(tmp_path: Path) -> None:
    # A hook that wedges every push when GitHub is unreachable is worse than
    # the problem it solves.
    got = _run(tmp_path, open_prs=[], closed_prs=[], authenticated=False)

    assert got.returncode == 0
    assert "gh missing or unauthenticated" in got.stderr


def test_an_unreachable_forge_never_blocks_a_push(tmp_path: Path) -> None:
    got = _run(tmp_path, open_prs=[], closed_prs=[], lookup_fails=True)

    assert got.returncode == 0
    assert "lookup failed" in got.stderr


@pytest.mark.parametrize(
    "remote_url",
    [
        "https://github.com/coilyco/agentic-os.git",
        "git@github.com:coilyco/agentic-os.git",
        "ssh://git@github.com/coilyco/agentic-os.git",
    ],
)
def test_every_github_remote_form_resolves_to_the_same_repo(
    tmp_path: Path, remote_url: str
) -> None:
    got = _run(tmp_path, open_prs=[_pr(1, merged=False, state="open")], closed_prs=[], remote_url=remote_url)

    assert got.returncode == 0, got.stderr
    log = (tmp_path / "bin" / "argv.log").read_text(encoding="utf-8")
    assert "--repo coilyco/agentic-os" in log
    assert "repos/coilyco/agentic-os/branches/aos/feature" in log


def test_a_remote_that_is_not_github_is_left_alone(tmp_path: Path) -> None:
    got = _run(
        tmp_path, open_prs=[], closed_prs=[], remote_url="https://git.example.invalid/o/r.git"
    )

    assert got.returncode == 0, got.stderr
    assert not (tmp_path / "bin" / "argv.log").exists()


def test_the_open_lookup_never_asks_for_state_all() -> None:
    # state=all returns the newest 100 PRs of any state, so on a busy repo an
    # older open PR falls off the page and a legitimate push is refused.
    text = GUARD.read_text(encoding="utf-8")

    # Only the two named states are ever requested, prose about state=all aside.
    calls = re.findall(r"branch_prs (\w+)", text)
    assert set(calls) == {"open", "merged"}, calls


# The lane half. This guard stood down on `merge-remote-main` (#1321), which is
# retired, so the stand-down is gone and these are its negative controls.


def test_the_retired_lane_no_longer_stands_the_guard_down(tmp_path: Path) -> None:
    got = _run(
        tmp_path, open_prs=[], closed_prs=[], branch="main", lane="merge-remote-main"
    )

    assert got.returncode == 1
    assert "refusing to push directly" in got.stderr


def test_the_refusal_says_no_lane_exempts_a_repo(tmp_path: Path) -> None:
    # The message has to explain why a slug that used to work no longer does,
    # or the next agent reads the refusal as a bug and reaches for --no-verify.
    got = _run(
        tmp_path, open_prs=[], closed_prs=[], branch="main", lane="merge-remote-main"
    )

    assert "No lane exempts a repo from this" in got.stderr
    assert "it is retired" in got.stderr


@pytest.mark.parametrize(
    "lane", ["pull-request", "pull-request-and-merge", "remote-branch-only", ""]
)
def test_every_other_lane_keeps_the_default_branch_guarded(
    tmp_path: Path, lane: str
) -> None:
    got = _run(tmp_path, open_prs=[], closed_prs=[], branch="main", lane=lane)

    assert got.returncode == 1
    assert "refusing to push directly to 'main'" in got.stderr


def test_an_undeclared_repo_is_told_which_lane_it_declares(tmp_path: Path) -> None:
    got = _run(tmp_path, open_prs=[], closed_prs=[], branch="main", lane="")

    assert "This repo declares no lane" in got.stderr


def test_an_unresolvable_reader_guards_and_says_so(tmp_path: Path) -> None:
    # Undeclared stays guarded, and so does unreadable.
    repo = tmp_path / "repo"
    repo.mkdir()
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
    empty = tmp_path / "empty-bin"
    empty.mkdir()
    # Bash absolute, PATH empty: no reader to find, and no git or sed either.
    got = subprocess.run(
        [shutil.which("bash") or "/bin/bash", str(GUARD)],
        cwd=repo,
        check=False,
        capture_output=True,
        text=True,
        env={
            "PATH": str(empty),
            "PRE_COMMIT_REMOTE_NAME": "origin",
            "PRE_COMMIT_REMOTE_URL": REMOTE_URL,
            "PRE_COMMIT_REMOTE_BRANCH": "refs/heads/main",
        },
    )

    assert got.returncode == 1
    assert "no lane reader found" in got.stderr


def test_the_pr_half_is_untouched_on_a_merge_remote_main_repo(tmp_path: Path) -> None:
    # The retirement changed the default-branch half only. A feature branch on a
    # repo that has not migrated yet is judged exactly as any other.
    got = _run(
        tmp_path,
        open_prs=[],
        closed_prs=[_pr(1030, merged=True, state="closed")],
        lane="merge-remote-main",
    )

    assert got.returncode == 1
    assert "already merged as PR #1030" in got.stderr
