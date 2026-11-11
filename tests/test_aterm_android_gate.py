"""The Android gate waits for the apk job only when a PR touches the build (COI-3634)."""

from __future__ import annotations

from pathlib import Path
import os
import subprocess

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "ci" / "aterm-android-gate.sh"

# A stand-in `gh`: files from FILES, one apk state per call from STATES.
SHIM = """#!/bin/sh
case "$*" in
  *pulls*files*) cat "$FILES" ;;
  *check-runs*)
    first=$(head -n1 "$STATES"); rest=$(tail -n +2 "$STATES")
    [ -n "$rest" ] && printf '%s\\n' "$rest" > "$STATES"
    printf '%s\\n' "$first" ;;
  *) exit 9 ;;
esac
"""


def _run(tmp_path: Path, files: list[str], states: list[str], **env: str) -> subprocess.CompletedProcess[str]:
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir(exist_ok=True)
    shim = bin_dir / "gh"
    shim.write_text(SHIM)
    shim.chmod(0o755)
    (tmp_path / "files").write_text("\n".join(files) + "\n")
    (tmp_path / "states").write_text("\n".join(states) + "\n")
    full = {
        **os.environ,
        "PATH": f"{bin_dir}:{os.environ['PATH']}",
        "FILES": str(tmp_path / "files"),
        "STATES": str(tmp_path / "states"),
        "GITHUB_REPOSITORY": "o/r",
        "PR_NUMBER": "1",
        "PR_HEAD_SHA": "abc",
        "ATERM_GATE_POLL": "0",
        "ATERM_GATE_ABSENT": "2",
        "ATERM_GATE_WAIT": "6",
        **env,
    }
    return subprocess.run(["bash", str(SCRIPT)], capture_output=True, text=True, check=False, env=full)


def test_a_pr_with_no_android_path_passes_without_asking_for_the_apk_job(tmp_path: Path) -> None:
    done = _run(tmp_path, ["docs/FEATURES.md", "aterm/daemon.go"], ["failure"])
    assert done.returncode == 0 and "no Android build path" in done.stdout


def test_an_android_path_with_a_green_apk_job_passes(tmp_path: Path) -> None:
    assert _run(tmp_path, ["aterm-android/src-tauri/src/lib.rs"], ["success"]).returncode == 0


def test_a_skipped_apk_job_passes(tmp_path: Path) -> None:
    assert _run(tmp_path, ["scripts/ci/aterm-android.sh"], ["skipped"]).returncode == 0


def test_a_red_apk_job_blocks_the_merge(tmp_path: Path) -> None:
    done = _run(tmp_path, ["aterm-android/plugins/tauri-plugin-speak/src/lib.rs"], ["failure"])
    assert done.returncode == 1 and "does not merge" in done.stdout


def test_a_pending_apk_job_is_waited_for(tmp_path: Path) -> None:
    done = _run(tmp_path, [".github/workflows/aterm-android.yml"], ["pending", "pending", "success"])
    assert done.returncode == 0


def test_no_apk_job_for_an_android_change_fails_after_the_grace(tmp_path: Path) -> None:
    done = _run(tmp_path, ["aterm-android/Cargo.toml"], ["absent"] * 8)
    assert done.returncode == 1 and "no apk job ran" in done.stdout


def test_a_sibling_script_that_is_not_the_android_build_does_not_match(tmp_path: Path) -> None:
    done = _run(tmp_path, ["scripts/ci/aterm-android-lane.sh.md", "scripts/ci/aterm_android_lane.py"], ["failure"])
    assert done.returncode == 0
