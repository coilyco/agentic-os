"""Refresh Homebrew metadata, then upgrade this estate's own formulae before the rest.

A bare `brew upgrade` resolves against whatever tap metadata is already on disk, so
it can report success while installing nothing: agentic-os#6831.
"""

from __future__ import annotations

import subprocess
import sys
from collections.abc import Callable, Sequence

TAP = "coilyco-flight-deck/tap"

RunCommand = Callable[..., subprocess.CompletedProcess[str]]


def _capture(argv: Sequence[str], runner: RunCommand) -> str:
    return runner(list(argv), check=True, capture_output=True, text=True).stdout


def _stream(argv: Sequence[str], runner: RunCommand) -> None:
    runner(list(argv), check=True)


def ours(runner: RunCommand) -> list[str]:
    """Formulae installed from our tap, read from brew rather than a tracked list.

    A hardcoded inventory is the defect this command exists to catch.
    """
    listed = _capture(["brew", "list", "--full-name", "--formula"], runner)
    return sorted(n for n in listed.split() if n.startswith(f"{TAP}/"))


def outdated(runner: RunCommand) -> list[str]:
    reported = _capture(["brew", "outdated", "--formula", "--verbose"], runner)
    return [line for line in reported.splitlines() if line.strip()]


def _try_upgrade(argv: Sequence[str], runner: RunCommand, out: Callable[[str], None]) -> bool:
    """Upgrade, reporting a failure rather than raising.

    One unbuildable formula in our own tap must not cost the caller every other
    update in the run: agentic-os#6835.
    """
    try:
        _stream(argv, runner)
        return True
    except subprocess.CalledProcessError as exc:
        out(f"FAILED: {' '.join(argv)} (exit {exc.returncode})")
        return False


def run(runner: RunCommand, out: Callable[[str], None]) -> int:
    _stream(["brew", "update"], runner)

    stale = outdated(runner)
    if stale:
        out("outdated after metadata refresh:")
        for line in stale:
            out(f"  {line}")
    else:
        out("nothing outdated after metadata refresh")

    failed = False
    mine = ours(runner)
    if mine:
        out(f"upgrading {len(mine)} formula(e) from {TAP}")
        failed |= not _try_upgrade(["brew", "upgrade", *mine], runner, out)
    else:
        out(f"no formulae installed from {TAP}")

    out("upgrading everything else")
    failed |= not _try_upgrade(["brew", "upgrade"], runner, out)

    remaining = outdated(runner)
    if failed or remaining:
        # Pinned or build-failed formulae survive an upgrade, so say so rather
        # than letting a clean exit imply everything moved.
        if remaining:
            out("still outdated after upgrade:")
            for line in remaining:
                out(f"  {line}")
        return 1
    out("all formulae current")
    return 0


def main() -> int:
    try:
        return run(subprocess.run, lambda line: print(line, flush=True))
    except FileNotFoundError:
        print("brew is not on PATH", file=sys.stderr)
        return 2
    except subprocess.CalledProcessError as exc:
        print(f"brew step failed: {' '.join(exc.cmd)}", file=sys.stderr)
        return exc.returncode or 1


if __name__ == "__main__":
    raise SystemExit(main())
