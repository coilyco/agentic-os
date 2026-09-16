"""The mirror step refuses a missing credential instead of skipping it.

`mirror-to-github` is the only thing in the pipeline that looks at the GitHub
mirror, and GitHub is the module origin, so a tag that never arrives is a
release no Go consumer can resolve. The step used to `exit 0` when the secret
was absent, which made a job named "Mirror to GitHub" go green having mirrored
nothing. See agentic-os#7797.
"""

from __future__ import annotations

import os
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "scripts" / "ci" / "mirror-to-github.sh"

# bash, matching the shebang and the workflow. Under dash the script dies at
# `set -o pipefail` before running a line, which macOS `sh` hides.


def _run(env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    base = {k: v for k, v in os.environ.items() if k != "PAT"}
    return subprocess.run(
        ["bash", str(SCRIPT), "mirror"],
        check=False,
        capture_output=True,
        text=True,
        env=base | (env or {}),
        cwd=ROOT,
    )


# The workflow always sets PAT, to "" when the secret is missing, so the empty
# case is the one that fires in CI. The unset case is a hand run.
@pytest.mark.parametrize("env", [{"PAT": ""}, {}], ids=["empty", "unset"])
def test_a_missing_credential_fails_the_step(env: dict[str, str]) -> None:
    result = _run(env)

    assert result.returncode == 1, (
        f"exit {result.returncode}: a missing PAT must fail the job. Exiting 0 "
        "reports a successful mirror of nothing"
    )
    assert "GITHUB_MIRROR_PAT" in result.stderr


def test_the_failure_says_nothing_was_mirrored() -> None:
    """An operator reading one CI line needs the consequence, not the cause."""
    assert "nothing was mirrored" in _run({"PAT": ""}).stderr


def test_an_unset_credential_is_not_a_bare_shell_error() -> None:
    """`set -u` would abort at the test itself, losing the explanation."""
    stderr = _run().stderr

    assert "unbound variable" not in stderr
    assert "::error::" in stderr


def test_a_present_credential_gets_past_the_guard(tmp_path: Path) -> None:
    """The five refusals above all pass if the guard rejects everything.

    Inverting `-z` to `-n` satisfies every one of them, so the set needs a case
    that must NOT be refused. Run from a directory that is not a repository, so
    the next step fails at `git remote add` locally: a real checkout would have
    it reach `git push` and authenticate against GitHub from CI.
    """
    result = subprocess.run(
        ["bash", str(SCRIPT), "mirror"],
        check=False,
        capture_output=True,
        text=True,
        env={k: v for k, v in os.environ.items() if k != "PAT"} | {"PAT": "unused"},
        cwd=tmp_path,
    )

    assert "GITHUB_MIRROR_PAT is empty" not in result.stderr, (
        "a supplied credential was rejected by the empty-credential guard"
    )


def test_an_unknown_verb_still_refuses() -> None:
    result = subprocess.run(
        ["bash", str(SCRIPT), "nonsense"], check=False, capture_output=True, text=True
    )

    assert result.returncode == 2
    assert "usage:" in result.stderr
