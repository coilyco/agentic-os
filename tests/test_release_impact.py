"""Exercise the artifact-level release impact classifier."""

from __future__ import annotations

from pathlib import Path

from agentic_os import release_impact
from agentic_os.release_impact import (
    is_aos_cli_release_input,
    is_aos_precommit_release_input,
    release_required,
)


ROOT = Path(__file__).resolve().parent.parent


def test_aos_cli_production_inputs_require_a_release() -> None:
    inputs = (
        "aos/main.go",
        "aos/role-harnesses.json",
        "agent-terminal/brand.go",
        "aosguard-release/main.go",
        ".specgen/guardfiles/aosguard/actions.kdl",
        "agentic_os/forgejo_actions_logs.py",
        "docker/dev-base/Dockerfile",
        "scripts/aos-release-build.sh",
        "scripts/render-aos-packaging.sh",
    )

    assert all(is_aos_cli_release_input(path) for path in inputs)
    assert release_required("aos-cli", inputs)


def test_aos_cli_non_artifact_changes_do_not_require_a_release() -> None:
    changes = (
        "AGENTS.md",
        "alacritty/alacritty.toml",
        "aos/main_test.go",
        "aos/native-checkout-repos.txt",
        "aos/role-personalities.json",
        "agent-terminal/testdata/director-overlay.json",
        "docs/aos-cli-release.md",
        "tests/test_aos_cli_release.py",
        ".forgejo/workflows/aos-cli-release.yml",
    )

    assert not any(is_aos_cli_release_input(path) for path in changes)
    assert not release_required("aos-cli", changes)


def test_aos_precommit_inputs_require_a_release() -> None:
    inputs = (
        ".pre-commit-hooks.yaml",
        "pyproject.toml",
        "agentic_os/config.py",
        "agentic_os/pre_commit/check_skill.py",
        "agentic_os/generators/generate_seed_skills.py",
        "agentic_os/seed_skills_data.py",
        "scripts/apply-agentic-os-hooks.py",
        "scripts/trufflehog-scan.sh",
    )

    assert all(is_aos_precommit_release_input(path) for path in inputs)
    assert release_required("aos-precommit", inputs)


def test_aos_precommit_non_artifact_changes_do_not_require_a_release() -> None:
    changes = (
        "AGENTS.md",
        ".pre-commit-config.yaml",
        "agentic_os/forgejo_actions_logs.py",
        "agentic_os/generators/generate_caps_reference.py",
        "docs/features-release-tooling.md",
        "tests/test_check_skill_low_context.py",
        ".forgejo/workflows/aos-precommit-release.yml",
    )

    assert not any(is_aos_precommit_release_input(path) for path in changes)
    assert not release_required("aos-precommit", changes)


def test_dev_base_uses_the_existing_affected_tier_contract() -> None:
    assert release_required("dev-base", ("docker/dev-base/install-common.sh",))
    assert release_required("dev-base", ("aos/main.go",))
    assert not release_required("dev-base", ("docs/dev-base-image.md",))
    assert not release_required("dev-base", ("alacritty/alacritty.toml",))


def test_changed_path_probe_does_not_collapse_renames(monkeypatch) -> None:
    commands: list[list[str]] = []

    class Probe:
        stdout = "aos/old.go\ndocs/old.go\n"

    def run(command, **_kwargs):
        commands.append(command)
        return Probe()

    monkeypatch.setattr(release_impact.subprocess, "run", run)

    assert release_impact._changed_paths("base", "head") == (
        "aos/old.go",
        "docs/old.go",
    )
    assert "--no-renames" in commands[0]


def test_automatic_release_workflows_use_the_owned_classifier() -> None:
    for workflow_name, surface in (
        ("aos-cli-release.yml", "aos-cli"),
        ("aos-precommit-release.yml", "aos-precommit"),
        ("dev-base-publish.yml", "dev-base"),
    ):
        workflow = (
            ROOT / ".forgejo" / "workflows" / workflow_name
        ).read_text(encoding="utf-8")
        assert "scripts/ci/release-impact.sh" in workflow
        assert surface in workflow
        assert "release_required == 'true'" in workflow

    bridge = (ROOT / "scripts" / "ci" / "release-impact.sh").read_text(
        encoding="utf-8"
    )
    assert "agentic_os.release_impact" in bridge
    assert "--base" in bridge
    assert "--force" in bridge
