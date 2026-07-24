"""Tests for the single full-image publish workflow."""

from __future__ import annotations

from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
PUBLISH = ROOT / ".forgejo" / "workflows" / "dev-base-publish.yml"
RELEASE = ROOT / ".forgejo" / "workflows" / "release.yml"
PUBLISH_ACTION = ROOT / "actions" / "publish-dev-base" / "action.yml"
VERIFY_FULL = (
    ROOT
    / "actions"
    / "publish-dev-base"
    / "scripts"
    / "verify-full.sh"
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
            "continue-on-error: true" in line for line in lines[index : index + 8]
        )


def test_workflows_publish_and_promote_only_full() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")
    release = RELEASE.read_text(encoding="utf-8")

    assert "publish-full:" in publish
    assert "retag-full:" in release
    assert "publish-languages" not in publish
    assert "retag-lang-" not in release
    assert "matrix:" not in publish
    assert "\n      tier:" not in publish
    assert "\n          tier:" not in release
    assert publish.count("uses: ./actions/publish-dev-base") == 1
    assert release.count("uses: ./actions/publish-dev-base") == 1


def test_publish_workflow_keeps_resume_inputs_and_non_blocking_alerts() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")
    release = RELEASE.read_text(encoding="utf-8")

    for needle in ("sha:", "tag:", "continue-on-error: true"):
        assert needle in publish
    for needle in ("sha:", "tag:", "source-tag:", "continue-on-error: true"):
        assert needle in release
    assert "tier:" not in publish
    assert "tier:" not in release
    _assert_alert_steps_are_non_blocking(publish)
    _assert_alert_steps_are_non_blocking(release)


def test_full_image_build_uses_the_dedicated_runner() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")

    assert publish.count("runs-on: docker-build") == 1
    assert "publish-full:" in publish
    assert "needs: [plan-draft]" in publish
    assert "plan-draft:\n    runs-on: docker\n" in publish


def test_publish_action_verifies_every_toolchain_and_aguard() -> None:
    action = PUBLISH_ACTION.read_text(encoding="utf-8")
    script = VERIFY_FULL.read_text(encoding="utf-8")

    assert "inputs:\n  tag:" in action
    assert "\n  tier:" not in action
    assert "Verify the full development surface on both architectures" in action
    assert "scripts/verify-full.sh" in action
    assert 'image="${IMAGE_BASE}:${TAG}"' in script
    assert "linux/amd64 linux/arm64" in script
    for command in (
        "node --version",
        "go version",
        "dotnet --list-sdks",
        "cargo --version",
        "python --version",
        "aguard --version",
    ):
        assert command in script
