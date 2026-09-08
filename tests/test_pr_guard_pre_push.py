"""Tests for the pre-push PR guard.

A branch whose PR already merged is still pushable on three of the four repos,
so the push succeeds, nothing points at the branch, and the work is stranded.
The forge is the only source that sees a squash or rebase merge, whose branch
tip is not an ancestor of the commit it merged as. See agentic-os#1034.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
GUARD = ROOT / "scripts" / "pr-guard-pre-push.sh"

BRANCH = "aos/feature"
REMOTE_URL = "https://forgejo.example.invalid/coilyco-flight-deck/agentic-os.git"


def _run(
    tmp_path: Path,
    *,
    open_prs: list,
    closed_prs: list,
    branch_http: str = "200",
    branch: str = BRANCH,
    lane: str | None = None,
):
    """Run the guard against a curl stub serving fixed PR listings.

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
    (bin_dir / "open.json").write_text(json.dumps(open_prs), encoding="utf-8")
    (bin_dir / "closed.json").write_text(json.dumps(closed_prs), encoding="utf-8")

    curl = bin_dir / "curl"
    curl.write_text(
        f"""#!/usr/bin/env bash
args="$*"
case "$args" in
  *"/branches/"*) printf '{branch_http}' ;;
  *"state=open"*) cat "{bin_dir}/open.json" ;;
  *"state=closed"*) cat "{bin_dir}/closed.json" ;;
  *) exit 1 ;;
esac
""",
        encoding="utf-8",
    )
    curl.chmod(0o755)

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
            "PRE_COMMIT_REMOTE_URL": REMOTE_URL,
            "PRE_COMMIT_REMOTE_BRANCH": f"refs/heads/{branch}",
            "FORGEJO_TOKEN": "t",
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
    got = _run(tmp_path, open_prs=[], closed_prs=[], branch_http="404")

    assert got.returncode == 0, got.stderr


def test_a_missing_token_never_blocks_a_push(tmp_path: Path) -> None:
    # A hook that wedges every push when Forgejo is unreachable is worse than
    # the problem it solves.
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    repo = tmp_path / "repo"
    repo.mkdir()
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
    env = {k: v for k, v in os.environ.items() if k != "FORGEJO_TOKEN"}
    got = subprocess.run(
        ["bash", str(GUARD)],
        cwd=repo,
        check=False,
        capture_output=True,
        text=True,
        env=env
        | {
            "PRE_COMMIT_REMOTE_NAME": "origin",
            "PRE_COMMIT_REMOTE_URL": REMOTE_URL,
            "PRE_COMMIT_REMOTE_BRANCH": f"refs/heads/{BRANCH}",
        },
    )

    assert got.returncode == 0
    assert "FORGEJO_TOKEN unset" in got.stderr


def test_an_unreachable_forge_never_blocks_a_push(tmp_path: Path) -> None:
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    curl = bin_dir / "curl"
    curl.write_text(
        '#!/usr/bin/env bash\ncase "$*" in *"/branches/"*) printf 200 ;; *) exit 7 ;; esac\n',
        encoding="utf-8",
    )
    curl.chmod(0o755)
    repo = tmp_path / "repo"
    repo.mkdir()
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
    got = subprocess.run(
        ["bash", str(GUARD)],
        cwd=repo,
        check=False,
        capture_output=True,
        text=True,
        env=os.environ
        | {
            "PATH": f"{bin_dir}{os.pathsep}{os.environ['PATH']}",
            "PRE_COMMIT_REMOTE_NAME": "origin",
            "PRE_COMMIT_REMOTE_URL": REMOTE_URL,
            "PRE_COMMIT_REMOTE_BRANCH": f"refs/heads/{BRANCH}",
            "FORGEJO_TOKEN": "t",
        },
    )

    assert got.returncode == 0
    assert "lookup failed" in got.stderr


def test_the_open_lookup_never_asks_for_state_all() -> None:
    # state=all returns the newest 100 PRs of any state, so on a busy repo an
    # older open PR falls off the page and a legitimate push is refused.
    text = GUARD.read_text(encoding="utf-8")

    assert "prs_on_branch open" in text
    assert "prs_on_branch closed" in text
    # Only the two named states are ever requested, prose about state=all aside.
    calls = re.findall(r"prs_on_branch (\w+)", text)
    assert set(calls) == {"open", "closed"}, calls



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
