"""Guard the Forgejo release actions against unbounded installs and API calls."""
from __future__ import annotations

from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
ACTION_SCRIPTS = [
    ROOT / "actions" / "create-release" / "run.sh",
    ROOT / "actions" / "upload-release-asset" / "run.sh",
    ROOT / "actions" / "bump-formula" / "run.sh",
]


def test_release_actions_source_shared_timeout_helpers() -> None:
    for path in ACTION_SCRIPTS:
        text = path.read_text(encoding="utf-8")
        assert 'source "${GITHUB_ACTION_PATH}/../_lib/release.sh"' in text
        assert "release_curl_status" in text
        assert "release_json_tool" in text


def test_release_actions_no_longer_self_install_jq_unbounded() -> None:
    for path in ACTION_SCRIPTS:
        text = path.read_text(encoding="utf-8")
        assert "apt-get update -qq && apt-get install -y -qq jq" not in text
        assert "apk add --no-cache jq" not in text


def test_release_action_helper_bounds_dependency_setup() -> None:
    text = (ROOT / "actions" / "_lib" / "release.sh").read_text(encoding="utf-8")
    assert (
        "timeout --preserve-status --kill-after=15s 2m sh -c 'apt-get update -qq && apt-get install -y -qq jq'"
        in text
    )
    assert "timeout --preserve-status --kill-after=15s 2m apk add --no-cache jq" in text
    assert "curl -sS --connect-timeout 10 --max-time 120" in text
