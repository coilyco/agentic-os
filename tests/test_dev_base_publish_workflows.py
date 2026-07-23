"""Tests for the tiered dev-base publish workflow surface."""
from __future__ import annotations

from pathlib import Path

from agentic_os.dev_base import PUBLISHED_TIER_NAMES


ROOT = Path(__file__).resolve().parent.parent
PUBLISH = ROOT / ".forgejo" / "workflows" / "dev-base-publish.yml"
RELEASE = ROOT / ".forgejo" / "workflows" / "release.yml"
DOCKER_BOOTSTRAP = ROOT / "actions" / "install-docker-buildx" / "action.yml"
PUBLISH_TIER = ROOT / "actions" / "publish-dev-base-tier" / "action.yml"


def _assert_alert_steps_are_non_blocking(text: str) -> None:
    lines = text.splitlines()
    alert_indexes = [
        i
        for i, line in enumerate(lines)
        if line.strip().startswith("- name: Alert Telegram on")
    ]
    assert alert_indexes
    for idx in alert_indexes:
        window = lines[idx : idx + 8]
        assert any("continue-on-error: true" in line for line in window)


def test_dev_base_publish_workflows_support_tier_reruns_and_non_blocking_alerts(
) -> None:
    publish = PUBLISH.read_text(encoding="utf-8")
    release = RELEASE.read_text(encoding="utf-8")

    for needle in (
        "sha:",
        "tier:",
        "tag:",
        "continue-on-error: true",
    ):
        assert needle in publish
    assert "Publish core image" in publish
    assert "scripts/dev-base-build.py" in publish
    assert "Probe core buildcache write" not in publish

    for needle in (
        "sha:",
        "tier:",
        "tag:",
        "source-tag:",
        "tag_name:",
        "release_sha:",
        "continue-on-error: true",
    ):
        assert needle in release

    assert "needs: [plan-release, retag-core]" in release
    assert (
        "needs: [plan-release, retag-lang-node, retag-lang-go, retag-lang-dotnet,"
        " retag-lang-rust, retag-lang-python]" in release
    )
    for retired_tier in ("ops", "agent"):
        assert f"publish-{retired_tier}" not in publish
        assert f"retag-{retired_tier}" not in release
        assert f"tier: {retired_tier}" not in publish
        assert f"tier: {retired_tier}" not in release
    assert "github.event.inputs.tier == 'all'" in release
    _assert_alert_steps_are_non_blocking(publish)
    _assert_alert_steps_are_non_blocking(release)


def test_dev_base_image_builds_use_the_dedicated_runner() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")

    assert publish.count("runs-on: docker-build") == len(PUBLISHED_TIER_NAMES)
    assert "plan-draft:\n    runs-on: docker\n" in publish


def test_docker_bootstrap_retries_every_download_and_is_shared() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")
    bootstrap = DOCKER_BOOTSTRAP.read_text(encoding="utf-8")
    tier = PUBLISH_TIER.read_text(encoding="utf-8")
    downloads = [line for line in bootstrap.splitlines() if "curl --retry" in line]

    assert len(downloads) == 3
    assert all("--retry-all-errors" in line and "-fsSL" in line for line in downloads)
    assert "uses: ./actions/install-docker-buildx" in publish
    assert "uses: ./actions/install-docker-buildx" in tier
