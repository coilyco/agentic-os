"""The build's google-services.json step: on from deployment config, else off (COI-3750)."""

from __future__ import annotations

import importlib.util
import json
import os
import shutil
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
APP_ID = "dev.coilyco.treehouse"
VALUES = "aterm-android/src-tauri/gen/android/app/src/main/res/values/treehouse_push.xml"

_spec = importlib.util.spec_from_file_location("aterm_google_services", ROOT / "scripts" / "ci" / "aterm_google_services.py")
assert _spec and _spec.loader
gs = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(gs)


def _config(package: str = APP_ID, **project: str) -> dict:
    return {
        "project_info": {"project_number": "123", "project_id": "demo-project", **project},
        "client": [
            {
                "client_info": {"mobilesdk_app_id": "1:123:android:other", "android_client_info": {"package_name": "com.other"}},
                "api_key": [{"current_key": "other-key"}],
            },
            {
                "client_info": {"mobilesdk_app_id": "1:123:android:abc", "android_client_info": {"package_name": package}},
                "api_key": [{"current_key": "fixture-key"}],
            },
        ],
    }


def test_the_client_for_this_app_is_picked() -> None:
    assert gs.resources(_config(), APP_ID) == {
        "google_app_id": "1:123:android:abc",
        "google_api_key": "fixture-key",
        "gcm_defaultSenderId": "123",
        "project_id": "demo-project",
    }


def test_a_storage_bucket_is_carried_when_present() -> None:
    assert gs.resources(_config(storage_bucket="b"), APP_ID)["google_storage_bucket"] == "b"


def test_a_config_without_this_app_is_refused_without_its_values() -> None:
    with pytest.raises(gs.ConfigError) as refused:
        gs.resources(_config(package="com.elsewhere"), APP_ID)
    assert APP_ID in str(refused.value) and "fixture-key" not in str(refused.value)


def test_values_are_escaped() -> None:
    assert "a&amp;b&lt;" in gs.render({"project_id": "a&b<"})


def _tree(tmp_path: Path) -> Path:
    script = tmp_path / "scripts" / "ci" / "aterm-android.sh"
    script.parent.mkdir(parents=True)
    for name in ("aterm-android.sh", "aterm_google_services.py"):
        shutil.copy(ROOT / "scripts" / "ci" / name, script.parent / name)
    return tmp_path


def _run(tree: Path, *args: str, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    clean = {k: v for k, v in os.environ.items() if not k.startswith("ATERM_ANDROID_")}
    return subprocess.run(
        ["bash", str(tree / "scripts" / "ci" / "aterm-android.sh"), *args],
        capture_output=True,
        text=True,
        check=False,
        env={**clean, **(env or {})},
    )


def test_no_config_leaves_push_off(tmp_path: Path) -> None:
    tree = _tree(tmp_path)
    result = _run(tree, "push-patch")
    assert result.returncode == 0, result.stderr
    assert "push is off" in result.stderr
    assert not (tree / VALUES).exists()


def test_a_deployment_config_turns_push_on(tmp_path: Path) -> None:
    tree = _tree(tmp_path)
    result = _run(tree, "push-patch", env={"ATERM_ANDROID_GOOGLE_SERVICES_JSON": json.dumps(_config())})
    assert result.returncode == 0, result.stderr
    xml = (tree / VALUES).read_text()
    assert '<string name="google_app_id" translatable="false">1:123:android:abc</string>' in xml
    assert "fixture-key" not in result.stdout + result.stderr


def test_a_broken_config_fails_the_build_and_leaves_no_file(tmp_path: Path) -> None:
    tree = _tree(tmp_path)
    result = _run(tree, "push-patch", env={"ATERM_ANDROID_GOOGLE_SERVICES_JSON": json.dumps(_config(package="com.elsewhere"))})
    assert result.returncode != 0
    assert not (tree / VALUES).exists()


def test_the_fixture_is_a_usable_config_and_yields_to_a_real_one(tmp_path: Path) -> None:
    tree = _tree(tmp_path)
    fixture = json.loads(_run(tree, "push-fixture").stdout)
    values = gs.resources(fixture, APP_ID)
    # Firebase refuses a key that is not 39 characters starting AIza.
    assert len(values["google_api_key"]) == 39 and values["google_api_key"].startswith("AIza")
    real = _run(tree, "push-patch", "fixture", env={"ATERM_ANDROID_GOOGLE_SERVICES_JSON": json.dumps(_config())})
    assert "from the deployment" in real.stdout
    assert "demo-project" in (tree / VALUES).read_text()


def test_a_release_build_refuses_the_fixture(tmp_path: Path) -> None:
    tree = _tree(tmp_path)
    result = _run(tree, "build", "1.2.3", str(tmp_path / "out.apk"), env={"ATERM_ANDROID_PUSH_FIXTURE": "1"})
    assert result.returncode != 0
    assert "debug builds only" in result.stderr
