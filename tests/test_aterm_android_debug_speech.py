"""Debug-only recognizers are in a debug APK and in no release APK (COI-3591)."""

from __future__ import annotations

from pathlib import Path
import os
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "ci" / "aterm-android.sh"
DEX_CLASS = b"Ldev/coilyco/treehouse/dictation/VoskRecognitionService;"


def _apk(tmp_path: Path, files: dict[str, bytes]) -> Path:
    apk = tmp_path / "app.apk"
    with zipfile.ZipFile(apk, "w") as zf:
        zf.writestr("AndroidManifest.xml", b"manifest")
        for name, data in files.items():
            zf.writestr(name, data)
    return apk


def _check(tmp_path: Path, apk: Path, want: str, manifest: str = "") -> subprocess.CompletedProcess[str]:
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir(exist_ok=True)
    shim = bin_dir / "aapt2"
    shim.write_text(f"#!/bin/sh\necho '{manifest}'\n")
    shim.chmod(0o755)
    env = {**os.environ, "PATH": f"{bin_dir}:{os.environ['PATH']}"}
    return subprocess.run(["bash", str(SCRIPT), "debug-speech", str(apk), want], capture_output=True, text=True, check=False, env=env)


def test_a_release_apk_with_no_recognizer_passes(tmp_path: Path) -> None:
    apk = _apk(tmp_path, {"classes.dex": b"Ldev/coilyco/treehouse/MainActivity;"})
    assert _check(tmp_path, apk, "absent").returncode == 0


def test_a_recognizer_class_in_dex_fails_a_release_check(tmp_path: Path) -> None:
    done = _check(tmp_path, _apk(tmp_path, {"classes.dex": DEX_CLASS}), "absent")
    assert done.returncode != 0 and "release build must not" in done.stderr


def test_a_native_library_fails_a_release_check(tmp_path: Path) -> None:
    done = _check(tmp_path, _apk(tmp_path, {"classes.dex": b"x", "lib/x86_64/libvosk.so": b"x"}), "absent")
    assert done.returncode != 0


def test_a_manifest_service_fails_a_release_check(tmp_path: Path) -> None:
    done = _check(tmp_path, _apk(tmp_path, {"classes.dex": b"x"}), "absent", "E: service (name=VoskRecognitionService)")
    assert done.returncode != 0


def test_the_debug_check_needs_the_recognizer_present(tmp_path: Path) -> None:
    assert _check(tmp_path, _apk(tmp_path, {"classes.dex": DEX_CLASS}), "present").returncode == 0
    done = _check(tmp_path, _apk(tmp_path, {"classes.dex": b"x"}), "present")
    assert done.returncode != 0 and "pass for nothing" in done.stderr


def test_the_dictation_plugins_recognizer_query_is_not_a_debug_recognizer(tmp_path: Path) -> None:
    query = "E: action (name=android.speech.RecognitionService)"
    assert _check(tmp_path, _apk(tmp_path, {"classes.dex": b"x"}), "absent", query).returncode == 0
