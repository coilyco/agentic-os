"""The python exec contract: what ships, and whose module wins.

agentic-os#6836 shipped a surface with no payload. #6837 let the caller's cwd
outrank the bundle.
"""

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
GUARDFILES = sorted((ROOT / ".umbra" / "guardfiles" / "aosguard").glob("*.kdl"))
MODULE_REF = re.compile(r'argv((?:\s+"[^"]*")+)')


def _module_argvs(text: str) -> list[list[str]]:
    return [
        re.findall(r'"([^"]*)"', match.group(1))
        for match in MODULE_REF.finditer(text)
        if "-m" in match.group(1)
    ]


@pytest.mark.parametrize("guardfile", GUARDFILES, ids=lambda p: p.name)
def test_every_module_exec_drops_the_callers_cwd(guardfile: Path) -> None:
    """A `-m` exec must pass -P, or the caller's cwd outranks the bundle."""
    for argv in _module_argvs(guardfile.read_text(encoding="utf-8")):
        assert "-P" in argv, f"{guardfile.name} runs {argv} without -P"
        assert argv.index("-P") < argv.index("-m"), f"{guardfile.name}: -P must precede -m"


def test_release_bundles_every_module_a_guardfile_execs() -> None:
    """The bundled list is derived, so a new verb cannot ship without its code."""
    listed = subprocess.run(
        ["sh", str(ROOT / "scripts" / "guardfile-python-modules.sh"), str(ROOT)],
        capture_output=True,
        text=True,
        check=True,
    ).stdout.split()
    bundled = {Path(line).name for line in listed}

    execed = {
        f"{argv[argv.index('-m') + 1].split('.')[-1]}.py"
        for guardfile in GUARDFILES
        for argv in _module_argvs(guardfile.read_text(encoding="utf-8"))
    }
    assert execed, "no module execs found; the parser stopped matching"
    assert execed <= bundled, f"guardfiles exec modules the bundle omits: {execed - bundled}"
    for name in bundled:
        assert (ROOT / "agentic_os" / name).is_file(), f"{name} is bundled but absent"


def test_a_planted_module_in_cwd_loses_to_the_bundle(tmp_path: Path) -> None:
    """The negative control: a decoy must be demonstrably ignored.

    Asserting only that the real module loads would pass for the wrong reason
    if nothing were planted at all.
    """
    decoy = tmp_path / "cwd" / "agentic_os"
    real = tmp_path / "bundled" / "agentic_os"
    for package in (decoy, real):
        package.mkdir(parents=True)
        (package / "__init__.py").touch()
    (decoy / "probe.py").write_text("print('DECOY')\n", encoding="utf-8")
    (real / "probe.py").write_text("print('BUNDLED')\n", encoding="utf-8")

    def run(*flags: str) -> str:
        return subprocess.run(
            [sys.executable, *flags, "-m", "agentic_os.probe"],
            cwd=decoy.parent,
            env={"PYTHONPATH": str(real.parent), "PATH": "/usr/bin:/bin"},
            capture_output=True,
            text=True,
        ).stdout.strip()

    assert run() == "DECOY", "the decoy is not actually shadowing; the control proves nothing"
    assert run("-P") == "BUNDLED"
