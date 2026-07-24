from __future__ import annotations

import os
import subprocess
from pathlib import Path


ROOT = Path(__file__).parents[1]
PROBE = ROOT / "scripts" / "aos-role-question.sh"


def _fake_aos(tmp_path: Path, response: str) -> tuple[Path, Path]:
    argv_log = tmp_path / "argv.log"
    fake = tmp_path / "aos"
    fake.write_text(
        "#!/bin/sh\n"
        'printf "%s\\n" "$@" >"$AOS_ARGV_LOG"\n'
        f"printf '%s\\n' {response!r}\n",
        encoding="utf-8",
    )
    fake.chmod(0o755)
    return fake, argv_log


def _run(
    tmp_path: Path,
    *args: str,
    response: str = "ROLE-CONFIRMED: designer",
) -> tuple[subprocess.CompletedProcess[str], str]:
    fake, argv_log = _fake_aos(tmp_path, response)
    env = {
        **os.environ,
        "AOS_BIN": str(fake),
        "AOS_ARGV_LOG": str(argv_log),
    }
    proc = subprocess.run(
        ["sh", str(PROBE), *args],
        cwd=ROOT,
        env=env,
        check=False,
        capture_output=True,
        text=True,
    )
    argv = argv_log.read_text(encoding="utf-8") if argv_log.exists() else ""
    return proc, argv


def test_cloud_question_selects_codex_and_accepts_role_marker(tmp_path: Path) -> None:
    proc, argv = _run(tmp_path, "cloud", "designer")

    assert proc.returncode == 0, proc.stderr
    assert "--image\nagentic-os:aos-local\n" in argv
    assert "--layout\ncodex\n" in argv
    assert "codex\nexec\n" in argv


def test_local_question_selects_goose_and_forwards_model(tmp_path: Path) -> None:
    proc, argv = _run(
        tmp_path,
        "local",
        "qa",
        "local-model",
        response="ROLE-CONFIRMED: QA",
    )

    assert proc.returncode == 0, proc.stderr
    assert "--image\nagentic-os:aos-local\n" in argv
    assert "--layout\ngoose\n" in argv
    assert "GOOSE_MODEL=local-model\n" in argv
    assert "goose\nrun\n--no-session\n" in argv


def test_question_accepts_display_name_marker(tmp_path: Path) -> None:
    proc, _ = _run(
        tmp_path,
        "local",
        "customer-success",
        response="ROLE-CONFIRMED: customer success",
    )

    assert proc.returncode == 0, proc.stderr


def test_question_rejects_missing_role_marker(tmp_path: Path) -> None:
    proc, _ = _run(tmp_path, "local", "designer", response="A design answer")

    assert proc.returncode == 1
    assert "response did not confirm designer" in proc.stderr


def test_question_rejects_unknown_role_before_launch(tmp_path: Path) -> None:
    proc, argv = _run(tmp_path, "local", "unknown")

    assert proc.returncode == 2
    assert "unknown role unknown" in proc.stderr
    assert argv == ""
