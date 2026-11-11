"""The restart guard blocks on a prompt or a draft, names the seat, ignores pending."""
from __future__ import annotations

import importlib.util
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def _load():
    spec = importlib.util.spec_from_file_location("aterm_restart_guard", ROOT / "scripts" / "aterm_restart_guard.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def seat(name: str, state: str = "idle", pending: int = 0, age: int = 5, drafting: bool = False) -> dict:
    return {"name": name, "state": state, "pending": pending, "state_seconds": age, "kai_drafting": drafting}


def test_a_prompt_blocks_and_names_the_seat_with_its_age() -> None:
    guard = _load()

    got = guard.blockers([seat("a"), seat("prod-director", "prompt", pending=14, age=8000)])

    assert got == ["prod-director state=prompt for 8000s pending=14"]


def test_a_draft_blocks_and_pending_alone_does_not() -> None:
    guard = _load()

    got = guard.blockers([seat("busy", "busy", pending=12), seat("typing", "idle", drafting=True, age=3)])

    assert got == ["typing state=idle for 3s pending=0 kai_drafting"]


def test_it_returns_zero_after_the_required_clear_reads_in_a_row() -> None:
    guard = _load()
    reads = iter([[seat("a", "prompt")], [seat("a")], [seat("a", "prompt")], [seat("a")], [seat("a")], [seat("a")]])
    said: list[str] = []

    code = guard.wait_until_clear(lambda: next(reads), reads=3, interval=0, timeout=999, say=said.append, sleep=lambda _s: None)

    assert code == 0
    assert said[-1].startswith("clear: 3 reads")
    assert sum(line.startswith("blocking:") for line in said) == 2


def test_it_gives_up_at_the_timeout_and_names_the_last_blocker() -> None:
    guard = _load()
    clock = iter(range(0, 100, 10))
    said: list[str] = []

    code = guard.wait_until_clear(
        lambda: [seat("prod-director", "prompt", age=60)],
        reads=3,
        interval=0,
        timeout=25,
        say=said.append,
        sleep=lambda _s: None,
        now=lambda: float(next(clock)),
    )

    assert code == 1
    assert said[-1] == "timeout, still blocking: prod-director state=prompt for 60s pending=0"
