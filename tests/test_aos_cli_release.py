"""Exercise the standalone aos release metadata contract."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess


ROOT = Path(__file__).resolve().parent.parent
TARGETS = ROOT / "aos" / "release-targets.txt"


def release_targets() -> list[str]:
    return [
        line.strip()
        for line in TARGETS.read_text(encoding="utf-8").splitlines()
        if line.strip() and not line.lstrip().startswith("#")
    ]


def artifact_name(target: str) -> str:
    goos, goarch = target.split("/")
    suffix = ".exe" if goos == "windows" else ""
    return f"aos-{goos}-{goarch}{suffix}"


def test_release_target_manifest_is_safe_and_unique() -> None:
    targets = release_targets()

    assert targets
    assert len(targets) == len(set(targets))
    assert all(
        re.fullmatch(r"[a-z0-9_-]+/[a-z0-9_-]+", target) for target in targets
    )


def test_packaging_covers_every_release_binary(tmp_path: Path) -> None:
    for target in release_targets():
        name = artifact_name(target)
        (tmp_path / name).write_bytes(f"fixture:{target}".encode())

    version = "aos-v1.2.3"
    env = os.environ | {
        "AOS_RELEASE_DIST": str(tmp_path),
        "AOS_RELEASE_VERSION": version,
    }
    subprocess.run(
        ["sh", str(ROOT / "scripts" / "render-aos-packaging.sh")],
        check=True,
        cwd=ROOT,
        env=env,
    )

    formula = (tmp_path / "aos.rb").read_text(encoding="utf-8")
    manifest = json.loads((tmp_path / "aos.json").read_text(encoding="utf-8"))
    rendered = formula + json.dumps(manifest)
    for target in release_targets():
        digest = hashlib.sha256((tmp_path / artifact_name(target)).read_bytes()).hexdigest()
        assert digest in rendered

    assert manifest["version"] == version.removeprefix("aos-v")
    assert f"/releases/download/{version}/" in rendered


def test_release_workflow_derives_assets_from_dist() -> None:
    workflow = (
        ROOT / ".forgejo" / "workflows" / "aos-cli-release.yml"
    ).read_text(encoding="utf-8")
    workflow_script = (ROOT / "scripts" / "ci" / "aos-cli-release.sh").read_text(
        encoding="utf-8"
    )
    builder = (ROOT / "scripts" / "aos-release-build.sh").read_text(
        encoding="utf-8"
    )

    assert "for asset in dist/*" in workflow_script
    assert "release-targets.txt" in builder
    assert "scripts/ci/aos-cli-release.sh" in workflow
    assert "ward exec aos-release-build" in workflow_script
    assert "ward exec aos-release-package" in workflow_script
