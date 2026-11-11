"""The launcher icons in the repo reach the APK, and a build without them fails (COI-3616)."""

from __future__ import annotations

from pathlib import Path
import importlib.util
import shutil
import struct
import subprocess
import zlib
import zipfile

ROOT = Path(__file__).resolve().parents[1]
ICONS = "aterm-android/src-tauri/icons/android"
RES = "aterm-android/src-tauri/gen/android/app/src/main/res"
DENSITIES = ("mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi")


def _tree(tmp_path: Path) -> Path:
    script = tmp_path / "scripts" / "ci" / "aterm-android.sh"
    script.parent.mkdir(parents=True)
    shutil.copy(ROOT / "scripts" / "ci" / "aterm-android.sh", script)
    shutil.copytree(ROOT / ICONS, tmp_path / ICONS)
    shutil.copy(ROOT / "scripts" / "ci" / "aterm_png_same.py", script.parent / "aterm_png_same.py")
    return tmp_path


def _run(tree: Path, verb: str, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["bash", str(tree / "scripts" / "ci" / "aterm-android.sh"), verb, *args],
        capture_output=True,
        text=True,
        check=False,
    )


def _apk(tree: Path, *, foreground: dict[str, bytes] | None = None, extra: dict[str, bytes] | None = None) -> Path:
    apk = tree / "app.apk"
    with zipfile.ZipFile(apk, "w") as zf:
        for density in DENSITIES:
            name = f"mipmap-{density}"
            data = (foreground or {}).get(density, (tree / ICONS / name / "ic_launcher_foreground.png").read_bytes())
            zf.writestr(f"res/{name}-v4/ic_launcher_foreground.png", data)
        for name, data in (extra or {}).items():
            zf.writestr(name, data)
    return apk


def test_our_icons_replace_the_templates_and_the_template_vectors_are_removed(tmp_path: Path) -> None:
    tree = _tree(tmp_path)
    res = tree / RES
    for rel in ("mipmap-xxxhdpi/ic_launcher_foreground.png", "values/ic_launcher_background.xml", "drawable-v24/ic_launcher_foreground.xml", "drawable/ic_launcher_background.xml"):
        (res / rel).parent.mkdir(parents=True, exist_ok=True)
        (res / rel).write_text("template")
    done = _run(tree, "icon-patch")
    assert done.returncode == 0, done.stderr
    for rel in ("mipmap-xxxhdpi/ic_launcher_foreground.png", "mipmap-anydpi-v26/ic_launcher.xml", "values/ic_launcher_background.xml"):
        assert (res / rel).read_bytes() == (tree / ICONS / rel).read_bytes(), rel
    assert not (res / "drawable-v24/ic_launcher_foreground.xml").exists()
    assert not (res / "drawable/ic_launcher_background.xml").exists()


def test_a_project_with_no_generated_res_is_refused(tmp_path: Path) -> None:
    done = _run(_tree(tmp_path), "icon-patch")
    assert done.returncode != 0
    assert "no generated res directory" in done.stderr


def test_an_apk_with_our_icons_passes_the_guard(tmp_path: Path) -> None:
    tree = _tree(tmp_path)
    done = _run(tree, "check-icon", str(_apk(tree)))
    assert done.returncode == 0, done.stderr


def test_an_apk_with_the_templates_icon_fails_the_guard_naming_the_file(tmp_path: Path) -> None:
    tree = _tree(tmp_path)
    done = _run(tree, "check-icon", str(_apk(tree, foreground={"xxxhdpi": b"tauri default"})))
    assert done.returncode != 0
    assert "xxxhdpi" in done.stderr


def test_an_apk_missing_a_density_or_carrying_the_template_vector_fails_the_guard(tmp_path: Path) -> None:
    tree = _tree(tmp_path)
    bare = tree / "bare.apk"
    with zipfile.ZipFile(bare, "w") as zf:
        zf.writestr("AndroidManifest.xml", b"x")
    missing = _run(tree, "check-icon", str(bare))
    assert missing.returncode != 0
    assert "no mdpi launcher foreground" in missing.stderr
    vector = _apk(tree, extra={"res/drawable-v24/ic_launcher_foreground.xml": b"<vector/>"})
    done = _run(tree, "check-icon", str(vector))
    assert done.returncode != 0
    assert "template's launcher foreground" in done.stderr


def _reencoded(png: Path) -> bytes:
    """The same picture as another encoder writes it, which is what a release build does to the icon."""
    spec = importlib.util.spec_from_file_location("aterm_png_same", ROOT / "scripts" / "ci" / "aterm_png_same.py")
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    width, height, data = module.pixels(png)
    raw = b"".join(b"\x00" + data[y * width * 4 : (y + 1) * width * 4] for y in range(height))

    def chunk(kind: bytes, body: bytes) -> bytes:
        return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body))

    return module.SIGNATURE + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b"")


def test_a_release_apk_that_re_encoded_the_same_icon_passes_and_a_different_picture_does_not(tmp_path: Path) -> None:
    tree = _tree(tmp_path)
    ours = tree / ICONS / "mipmap-xxxhdpi" / "ic_launcher_foreground.png"
    recoded = _reencoded(ours)
    assert recoded != ours.read_bytes()
    assert _run(tree, "check-icon", str(_apk(tree, foreground={"xxxhdpi": recoded}))).returncode == 0
    other = tree / ICONS / "mipmap-xxxhdpi" / "ic_launcher.png"
    assert _run(tree, "check-icon", str(_apk(tree, foreground={"xxxhdpi": _reencoded(other)}))).returncode != 0
