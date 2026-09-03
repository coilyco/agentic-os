"""Tests for the sealed Homebrew update bridge."""

from __future__ import annotations

import importlib.util
import subprocess
from pathlib import Path


def _load_brew_module():
    source = (
        Path(__file__).resolve().parents[1]
        / ".umbra"
        / "guardfiles"
        / "aosguard"
        / "brew_update.py"
    )
    spec = importlib.util.spec_from_file_location("aosguard_brew_update", source)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load brew module from {source}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


brew_update = _load_brew_module()


class _Runner:
    """Records argv and replays canned stdout for the capturing calls."""

    def __init__(self, listed: str = "", outdated_stdout: list[str] | None = None):
        self.calls: list[list[str]] = []
        self.listed = listed
        self.outdated_stdout = outdated_stdout or [""]
        self._outdated_seen = 0

    def __call__(self, argv, **kwargs):
        self.calls.append(list(argv))
        stdout = ""
        if argv[:2] == ["brew", "list"]:
            stdout = self.listed
        elif argv[:2] == ["brew", "outdated"]:
            index = min(self._outdated_seen, len(self.outdated_stdout) - 1)
            stdout = self.outdated_stdout[index]
            self._outdated_seen += 1
        return subprocess.CompletedProcess(argv, 0, stdout=stdout, stderr="")


def test_our_formulae_come_from_brew_not_a_tracked_list():
    runner = _Runner(
        listed="jq\ncoilyco-flight-deck/tap/aos\nripgrep\ncoilyco-flight-deck/tap/umbra\n"
    )
    assert brew_update.ours(runner) == [
        "coilyco-flight-deck/tap/aos",
        "coilyco-flight-deck/tap/umbra",
    ]


def test_metadata_refresh_precedes_every_read_and_upgrade():
    runner = _Runner(listed="coilyco-flight-deck/tap/aos\n")
    brew_update.run(runner, lambda _line: None)
    assert runner.calls[0] == ["brew", "update"]


def test_our_tap_upgrades_before_the_general_upgrade():
    runner = _Runner(listed="coilyco-flight-deck/tap/aos\n")
    brew_update.run(runner, lambda _line: None)
    upgrades = [c for c in runner.calls if c[:2] == ["brew", "upgrade"]]
    assert upgrades == [
        ["brew", "upgrade", "coilyco-flight-deck/tap/aos"],
        ["brew", "upgrade"],
    ]


def test_no_tap_formulae_skips_the_targeted_upgrade():
    runner = _Runner(listed="jq\nripgrep\n")
    brew_update.run(runner, lambda _line: None)
    upgrades = [c for c in runner.calls if c[:2] == ["brew", "upgrade"]]
    assert upgrades == [["brew", "upgrade"]]


def test_formula_left_outdated_after_upgrade_is_a_nonzero_exit():
    runner = _Runner(
        listed="coilyco-flight-deck/tap/aos\n",
        outdated_stdout=[
            "coilyco-flight-deck/tap/aos (0.296.0) < 0.297.0",
            "coilyco-flight-deck/tap/aos (0.296.0) < 0.297.0",
        ],
    )
    assert brew_update.run(runner, lambda _line: None) == 1


def test_everything_current_after_upgrade_is_a_zero_exit():
    runner = _Runner(
        listed="coilyco-flight-deck/tap/aos\n",
        outdated_stdout=["coilyco-flight-deck/tap/aos (0.296.0) < 0.297.0", ""],
    )
    assert brew_update.run(runner, lambda _line: None) == 0


def test_missing_brew_reports_rather_than_traces(monkeypatch, capsys):
    def _absent(*_args, **_kwargs):
        raise FileNotFoundError("brew")

    monkeypatch.setattr(brew_update.subprocess, "run", _absent)
    assert brew_update.main() == 2
    assert "brew is not on PATH" in capsys.readouterr().err
