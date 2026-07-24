"""Tests for the independent dev-base publish workflow surface."""
from __future__ import annotations

from pathlib import Path

from agentic_os.dev_base import PUBLISHED_TIER_NAMES


ROOT = Path(__file__).resolve().parent.parent
PUBLISH = ROOT / ".forgejo" / "workflows" / "dev-base-publish.yml"
RELEASE = ROOT / ".forgejo" / "workflows" / "release.yml"
PUBLISH_TIER = ROOT / "actions" / "publish-dev-base-tier" / "action.yml"
VERIFY_RUST = (
    ROOT
    / "actions"
    / "publish-dev-base-tier"
    / "scripts"
    / "verify-rust.sh"
)


def _assert_alert_steps_are_non_blocking(text: str) -> None:
    lines = text.splitlines()
    alert_indexes = [
        index
        for index, line in enumerate(lines)
        if line.strip().startswith("- name: Alert Telegram on")
    ]
    assert alert_indexes
    for index in alert_indexes:
        assert any(
            "continue-on-error: true" in line
            for line in lines[index : index + 8]
        )


def test_publish_and_release_workflows_have_no_core_job() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")
    release = RELEASE.read_text(encoding="utf-8")

    assert "publish-core" not in publish
    assert "retag-core" not in release
    assert "tier: core" not in publish
    assert "tier: core" not in release
    assert publish.count("uses: ./actions/publish-dev-base-tier") == 2
    assert release.count("uses: ./actions/publish-dev-base-tier") == len(
        PUBLISHED_TIER_NAMES
    )


def test_language_builds_match_runner_capacity_and_full_waits_for_the_matrix() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")
    release = RELEASE.read_text(encoding="utf-8")

    for tier in PUBLISHED_TIER_NAMES:
        if not tier.startswith("lang-"):
            continue
        assert f"          - {tier}" in publish
        assert f"retag-{tier}:\n    needs: [plan-release]" in release

    assert "publish-languages:\n    name: publish-${{ matrix.tier }}" in publish
    assert "max-parallel: 4" in publish
    assert "tier: ${{ matrix.tier }}" in publish
    assert "needs: [plan-draft, publish-languages]" in publish
    assert (
        "needs: [plan-release, retag-lang-go, retag-lang-dotnet, "
        "retag-lang-rust, retag-lang-python]" in release
    )


def test_publish_workflow_keeps_resume_inputs_and_non_blocking_alerts() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")
    release = RELEASE.read_text(encoding="utf-8")

    for needle in ("sha:", "tier:", "tag:", "continue-on-error: true"):
        assert needle in publish
    for needle in (
        "sha:",
        "tier:",
        "tag:",
        "source-tag:",
        "continue-on-error: true",
    ):
        assert needle in release

    assert "github.event.inputs.tier == 'all'" in release
    _assert_alert_steps_are_non_blocking(publish)
    _assert_alert_steps_are_non_blocking(release)


def test_each_image_build_uses_the_dedicated_runner() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")
    assert publish.count("runs-on: docker-build") == 2
    assert "publish-languages:" in publish
    assert "publish-full:" in publish
    assert "plan-draft:\n    runs-on: docker\n" in publish


def test_rust_publish_verifies_the_prefixed_specialist_image() -> None:
    action = PUBLISH_TIER.read_text(encoding="utf-8")
    script = VERIFY_RUST.read_text(encoding="utf-8")

    assert "Verify lang-rust native development surface on both architectures" in action
    assert "if: ${{ inputs.tier == 'lang-rust' }}" in action
    assert "scripts/verify-rust.sh" in action
    assert 'image="${IMAGE_BASE}:lang-rust-${TAG}"' in script
    assert "linux/amd64 linux/arm64" in script
    assert "pkg-config --exists wayland-client xkbcommon" in script
    assert "rustup target list --installed" in script
