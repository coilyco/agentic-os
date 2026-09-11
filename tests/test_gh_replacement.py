"""Static and behavioral contract for the guarded `gh` replacement binary."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parents[1]
PROJECT = ROOT / ".umbra" / "guardfiles"
MEMBER = PROJECT / "gh" / "gh.kdl"

# The writes that must never reach the mirror. `create` is the one Kai asked
# for; the rest decide the same pull request on the same copy.
WITHHELD = ("create", "merge", "edit", "close", "reopen", "ready", "review", "comment")


@pytest.fixture(scope="module")
def shim(tmp_path_factory: pytest.TempPathFactory) -> Path:
    suffix = ".exe" if os.name == "nt" else ""
    binary = tmp_path_factory.mktemp("gh-shim") / f"gh{suffix}"
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


def test_a_granted_read_reaches_the_real_binary(shim: Path, fake_gh: Path) -> None:
    result = run(shim, fake_gh, "pr", "list", "--state", "open")

    assert result.returncode == 0, result.stdout + result.stderr
    assert "REAL_GH pr list --state open" in result.stdout


def test_an_ungranted_verb_is_refused_rather_than_passed_through(
    shim: Path, fake_gh: Path
) -> None:
    result = run(shim, fake_gh, "workflow", "run", "x")

    assert result.returncode == 2
    assert "REAL_GH" not in result.stdout


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


def test_the_guardfile_grants_no_pull_request_write(shim: Path) -> None:
    """A static read, so a `can run` added beside a withhold fails here first."""
    source = MEMBER.read_text(encoding="utf-8")
    for verb in WITHHELD:
        assert f'can run "pr {verb}"' not in source
        assert f"withhold pr {verb} {{" in source


def test_the_replacement_declares_itself(shim: Path, fake_gh: Path) -> None:
    """UMBRA_IDENTIFY is the only surface that says what stands on the name."""
    env = os.environ.copy()
    env["PATH"] = f"{fake_gh}{os.pathsep}{env['PATH']}"
    env["UMBRA_IDENTIFY"] = "1"
    result = subprocess.run(
        [str(shim)], capture_output=True, text=True, env=env, check=False
    )

    assert "umbra replacement" in result.stdout + result.stderr
