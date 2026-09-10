"""Tests for the issue-ref Stop hook.

The retired hook read the transcript and got an empty string on 94.7% of 5,298
events, because at Stop entry the final assistant text has not been flushed yet.
This one reads `last_assistant_message` off the payload, which carried the exact
text on 11 of 11 measured events. Rates and method:
teable:coilyco-flight-deck/agentic-os#7246.
"""

from __future__ import annotations

import json
import statistics
import subprocess
import time
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
HOOK = ROOT / "scripts" / "issue-ref-links.sh"

READER = "https://claude.ai/code/artifact/f6de33ad-e4d9-4a46-9de6-bef8d986e8e6"

# From the replay over 853 transcripts: each of these must stay silent, because
# a hook that fires on the convention itself cannot be quoted in a reply.
NEGATIVES = [
    "teable:<owner>/<repo>#<n>",
    "the `teable:<owner>/<repo>#<n>` form",
    "the #N form",
    "step #1",
    "color #3ed7a9",
    "#define FOO",
]


def _run(tmp_path: Path, message: str, *, mode: str = "warn", active: bool = False):
    log = tmp_path / "issue-ref-links.log"
    payload = json.dumps(
        {
            "stop_hook_active": active,
            "transcript_path": "",
            "last_assistant_message": message,
        }
    )
    proc = subprocess.run(
        ["bash", str(HOOK)],
        input=payload,
        capture_output=True,
        text=True,
        env={
            "PATH": "/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin",
            "AOS_ISSUE_REF_LOG": str(log),
            "AOS_ISSUE_REF_MODE": mode,
        },
    )
    assert proc.returncode == 0, proc.stderr
    logged = log.read_text().strip() if log.exists() else ""
    return proc.stdout.strip(), logged


def _decision(stdout: str):
    return json.loads(stdout) if stdout else None


def _reason(stdout: str) -> str:
    decision = _decision(stdout)
    assert decision is not None, "expected a block decision"
    return decision["reason"]


@pytest.mark.parametrize("message", NEGATIVES)
def test_negative_controls_stay_silent(tmp_path, message):
    stdout, logged = _run(tmp_path, message, mode="block")
    assert _decision(stdout) is None
    assert "reason=no-refs" in logged


def test_bare_ref_in_prose_fires(tmp_path):
    _, logged = _run(tmp_path, "see PR #1573 for context")
    assert "ref=#1573|ambiguous|prose" in logged


def test_bare_ref_inside_inline_code_still_fires(tmp_path):
    """The inline exemption hid a ref in 18.8% of ref-bearing messages, and Kai's
    own complaint was written as a backticked #1234, so it is gone. `where=code`
    records the placement the block decision needs without exempting it."""
    _, logged = _run(tmp_path, "see `#1234` for context")
    assert "ref=#1234|ambiguous|code" in logged


def test_fenced_block_stays_exempt(tmp_path):
    stdout, logged = _run(
        tmp_path, "text\n```\nsee #1234 here\n```\ndone", mode="block"
    )
    assert _decision(stdout) is None
    assert "reason=no-refs" in logged


def test_teable_ref_targets_the_record_reader(tmp_path):
    stdout, _ = _run(
        tmp_path, "filed teable:coilyco-flight-deck/agentic-os#7244", mode="block"
    )
    reason = _reason(stdout)
    # Both forms: the page reads the hash first and falls back to ?n=, and
    # whether a viewer forwards a query string into the frame is not guaranteed.
    assert f"{READER}?n=7244#7244" in reason


def test_bare_teable_ref_targets_the_record_reader(tmp_path):
    """`teable:#<n>` is the honest citation when the number is known and the org
    is not, and `seq` is global so the number alone resolves."""
    stdout, logged = _run(tmp_path, "cite teable:#7246", mode="block")
    assert f"{READER}?n=7246#7246" in _reason(stdout)
    assert "|record-bare|" in logged


def test_forgejo_ref_targets_a_pull_request(tmp_path):
    stdout, _ = _run(tmp_path, "opened coilyco-bridge/agentic-os-kai#978", mode="block")
    reason = _reason(stdout)
    assert "https://forgejo.coilysiren.me/coilyco-bridge/agentic-os-kai/pulls/978" in reason


def test_ambiguous_ref_asks_rather_than_guesses(tmp_path):
    """A wrong org does not 404. It resolves to a real unrelated row, which
    docs/teable-tracker.md records as the worse failure."""
    reason = _reason(_run(tmp_path, "want me to take #1234?", mode="block")[0])
    assert "ambiguous" in reason
    assert "teable:#1234" in reason


def test_reason_never_names_a_forgejo_issue_url(tmp_path):
    """Forgejo issue trackers are off fleet-wide. The retired hook's reason string
    demanded /issues/<N>, so reviving it verbatim would have made every agent on
    the fleet emit a dead link."""
    message = "teable:coilyco-flight-deck/agentic-os#7246 and coilyco-bridge/x#12 and #1234"
    reason = _reason(_run(tmp_path, message, mode="block")[0])
    assert "/issues/" not in reason


def test_loop_guard_lets_an_unfixable_message_through(tmp_path):
    stdout, logged = _run(tmp_path, "see #1234", mode="block", active=True)
    assert _decision(stdout) is None
    assert "reason=loop-guard" in logged


def test_warn_mode_logs_without_blocking(tmp_path):
    stdout, logged = _run(tmp_path, "see #1234 and coilyco-bridge/x#77")
    assert stdout == ""
    assert "decision=warn" in logged
    assert "refs=2" in logged
    assert "ambiguous=1" in logged


def test_empty_message_is_recorded_as_such(tmp_path):
    """The sight condition reads off this line: an empty rate at or under 5%
    against the retired hook's 94.7%."""
    _, logged = _run(tmp_path, "")
    assert "reason=empty-message" in logged


def test_mode_off_writes_nothing(tmp_path):
    stdout, logged = _run(tmp_path, "see #1234", mode="off")
    assert stdout == ""
    assert logged == ""


def test_hook_does_not_stall_the_turn(tmp_path):
    """Guards the regression that made the cost condition worth writing down: a
    `sleep 1` per turn end. The acceptance figures (p50 100 ms, p95 400 ms) come
    from elapsed_ms in the warn log on live traffic, since a shared CI runner
    cannot hold a bound that tight."""
    message = "landed teable:coilyco-flight-deck/agentic-os#7246 and #1234"
    samples = []
    for _ in range(20):
        start = time.perf_counter()
        _run(tmp_path, message)
        samples.append((time.perf_counter() - start) * 1000)
    assert statistics.median(samples) < 400
    assert max(samples) < 1000
