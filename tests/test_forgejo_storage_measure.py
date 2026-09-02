"""Tests for the sealed Forgejo storage measurement bridge."""

from __future__ import annotations

import importlib.util
import subprocess
import sys
from pathlib import Path

import pytest


def _load_measurement_module():
    source = (
        Path(__file__).resolve().parents[1]
        / ".umbra"
        / "guardfiles"
        / "aosguard"
        / "forgejo_storage_measure.py"
    )
    spec = importlib.util.spec_from_file_location(
        "aosguard_forgejo_storage_measure", source
    )
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load measurement module from {source}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


measure = _load_measurement_module()


class RecordingRunner:
    def __init__(self, *, fail_at: int | None = None, timeout_at: int | None = None):
        self.calls: list[list[str]] = []
        self.fail_at = fail_at
        self.timeout_at = timeout_at

    def __call__(
        self,
        argv: list[str],
        *,
        check: bool,
        timeout: int,
    ) -> subprocess.CompletedProcess[bytes]:
        assert check is False
        assert timeout == measure.MEASUREMENT_TIMEOUT_SECONDS
        index = len(self.calls)
        self.calls.append(argv)
        if index == self.timeout_at:
            raise subprocess.TimeoutExpired(argv, timeout)
        return subprocess.CompletedProcess(argv, 7 if index == self.fail_at else 0)


def test_measurement_uses_only_fixed_kubectl_targets() -> None:
    runner = RecordingRunner()

    assert measure.main([], runner=runner) == 0
    assert len(runner.calls) == len(measure.MEASUREMENTS) + 2
    assert runner.calls[0] == ["kubectl", "config", "current-context"]
    assert runner.calls[1] == [
        "kubectl",
        "-n",
        "forgejo",
        "get",
        "pvc,pods",
        "-o",
        "wide",
    ]

    allowed_targets = {measure.FORGEJO_TARGET, measure.FORGEJO_DB_TARGET}
    for argv in runner.calls[2:]:
        assert argv[:4] == ["kubectl", "-n", "forgejo", "exec"]
        assert argv[4] in allowed_targets
        assert argv[5] == "--"


@pytest.mark.parametrize("failure", ["exit", "timeout"])
def test_measurement_continues_after_an_incomplete_section(failure: str) -> None:
    runner = RecordingRunner(
        fail_at=3 if failure == "exit" else None,
        timeout_at=3 if failure == "timeout" else None,
    )

    assert measure.main([], runner=runner) == 1
    assert len(runner.calls) == len(measure.MEASUREMENTS) + 2


def test_measurement_accepts_no_operational_arguments() -> None:
    with pytest.raises(SystemExit):
        measure.main(["--namespace", "other"], runner=RecordingRunner())
