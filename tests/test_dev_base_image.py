"""Tests for the single full dev-base image contract."""

from __future__ import annotations

import argparse
import importlib.util
from pathlib import Path

import pytest

from agentic_os.dev_base import (
    DEV_BASE_ROOT,
    PUBLISHED_TIER_NAMES,
    REGISTRY_BASE,
    TIER_BY_NAME,
    publish_plan,
    tier_tag,
)


REPO_ROOT = Path(__file__).resolve().parent.parent
SCRIPT = REPO_ROOT / "scripts" / "dev-base-build.py"
DOCKERFILE = DEV_BASE_ROOT / "Dockerfile"


def _load_script():
    spec = importlib.util.spec_from_file_location("dev_base_build", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_only_the_full_image_is_publishable() -> None:
    assert PUBLISHED_TIER_NAMES == ("full",)
    assert tuple(TIER_BY_NAME) == ("full",)
    assert TIER_BY_NAME["full"].dockerfile == DOCKERFILE
    assert not (DEV_BASE_ROOT / "full" / "Dockerfile").exists()

    with pytest.raises(ValueError, match="unsupported dev-base image"):
        tier_tag("lang-node", "candidate")


def test_full_dockerfile_contains_every_language_and_operator_surface() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8")

    assert text.count(" AS dev-base-full") == 1
    assert "dev-base-lang-" not in text
    assert text.count("bash /tmp/install-common.sh") == 1
    for command in (
        "node --version",
        "go version",
        "dotnet --list-sdks",
        "cargo --version",
        "trunk --version",
        "python --version",
        "pipenv --version",
        "aguard --version",
    ):
        assert command in text
    assert "specgen-linux-${TARGETARCH}" in text
    assert "sha256sum -c -" in text
    assert "COPY --from=aguard-spec" in text
    assert "COPY --from=aguard-python" in text


def test_version_defaults_have_one_owning_source() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8")
    for name in (
        "UV_VERSION",
        "GO_VERSION",
        "DOTNET_VERSION",
        "TRUNK_VERSION",
        "SPECGEN_VERSION",
        "NODE_VERSION",
        "GOLANGCI_LINT_VERSION",
        "TRUFFLEHOG_VERSION",
        "KDLFMT_VERSION",
    ):
        assert text.count(f"ARG {name}=") == 1


def test_publish_plan_has_one_plain_tagged_image() -> None:
    plan = publish_plan(REGISTRY_BASE, "candidate", ("release", "latest"))

    assert plan == [
        {
            "tier": "full",
            "stage": "dev-base-full",
            "dockerfile": "docker/dev-base/Dockerfile",
            "context_dir": "docker/dev-base",
            "image": f"{REGISTRY_BASE}:candidate",
            "cache_image": f"{REGISTRY_BASE}:buildcache",
            "alias_images": [
                f"{REGISTRY_BASE}:release",
                f"{REGISTRY_BASE}:latest",
            ],
            "alias_image": f"{REGISTRY_BASE}:release",
        }
    ]


def test_local_build_targets_full_with_all_named_contexts(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_host_targetarch", lambda: "amd64")
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "commit")

    script._build_plan(
        REGISTRY_BASE,
        "candidate",
        False,
        "linux/amd64,linux/arm64",
        only_tier="full",
    )

    assert len(commands) == 1
    build = commands[0]
    assert build[:3] == ["docker", "build", "--build-arg"]
    assert "TARGETARCH=amd64" in build
    assert "WARD_CONFIG_REF_COMMIT=commit" in build
    assert "aos-cli=aos" in build
    assert "aguard-spec=.specgen" in build
    assert "aguard-python=agentic_os" in build
    assert build[build.index("--target") + 1] == "dev-base-full"
    assert Path(build[-1]).as_posix() == "docker/dev-base"


def test_pushed_build_skips_an_existing_checkpoint(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []
    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: True)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "commit")

    script._build_plan(
        REGISTRY_BASE,
        "candidate",
        True,
        "linux/amd64,linux/arm64",
        ("release",),
        "full",
    )

    assert commands == []


def test_promote_plan_retags_a_draft_to_release_aliases(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    class Probe:
        returncode = 0
        stdout = ""
        stderr = ""

    monkeypatch.setattr(script.subprocess, "run", lambda *args, **kwargs: Probe())
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))

    script._promote_plan(
        REGISTRY_BASE,
        "draft-commit",
        "candidate",
        ("release", "latest"),
        "full",
    )

    create = next(
        cmd
        for cmd in commands
        if cmd[:4] == ["docker", "buildx", "imagetools", "create"]
    )
    assert create == [
        "docker",
        "buildx",
        "imagetools",
        "create",
        "-t",
        f"{REGISTRY_BASE}:candidate",
        "-t",
        f"{REGISTRY_BASE}:release",
        "-t",
        f"{REGISTRY_BASE}:latest",
        f"{REGISTRY_BASE}:draft-commit",
    ]


def test_manifest_inspect_retries_and_reports_the_digest(monkeypatch, tmp_path) -> None:
    script = _load_script()
    sleeps: list[int] = []

    class Probe:
        def __init__(self, returncode: int, stdout: str, stderr: str) -> None:
            self.returncode = returncode
            self.stdout = stdout
            self.stderr = stderr

    probes = iter(
        [
            Probe(1, "", "manifest unknown"),
            Probe(0, "Digest: sha256:abc123", ""),
        ]
    )
    monkeypatch.setattr(script.subprocess, "run", lambda *args, **kwargs: next(probes))
    monkeypatch.setattr(script.time, "sleep", lambda seconds: sleeps.append(seconds))
    summary = tmp_path / "summary.md"
    monkeypatch.setenv("GITHUB_STEP_SUMMARY", str(summary))

    assert script._inspect_manifest(f"{REGISTRY_BASE}:candidate") == "sha256:abc123"
    assert sleeps == [1]
    assert "succeeded after attempt 2/3" in summary.read_text(encoding="utf-8")


def test_cmd_check_reports_checkpoint_state(monkeypatch) -> None:
    script = _load_script()
    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: True)
    monkeypatch.setattr(script, "_target_matches_source", lambda *_args: True)

    build_args = argparse.Namespace(
        registry=REGISTRY_BASE,
        tag="candidate",
        alias=["release"],
        mode="build",
        tier="full",
        source_tag="",
    )
    promote_args = argparse.Namespace(
        registry=REGISTRY_BASE,
        tag="candidate",
        alias=["release", "latest"],
        mode="promote",
        tier="full",
        source_tag="draft-commit",
    )

    assert script._cmd_check(build_args) == 0
    assert script._cmd_check(promote_args) == 0
