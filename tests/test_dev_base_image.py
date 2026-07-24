"""Tests for the independent dev-base language image contract."""
from __future__ import annotations

import argparse
import importlib.util
import re
from pathlib import Path

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
FULL_DOCKERFILE = DEV_BASE_ROOT / "full" / "Dockerfile"
LANGUAGE_TIERS = tuple(
    tier for tier in PUBLISHED_TIER_NAMES if tier.startswith("lang-")
)


def _load_script():
    spec = importlib.util.spec_from_file_location("dev_base_build", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _stage_text(text: str, stage: str) -> str:
    match = re.search(rf"^FROM ubuntu:\S+ AS {re.escape(stage)}$", text, re.MULTILINE)
    assert match is not None
    start = match.start()
    remainder = text[start:]
    next_stage = remainder.find("\nFROM ", 1)
    return remainder if next_stage < 0 else remainder[:next_stage]


def test_core_image_is_removed_from_source_and_publication() -> None:
    assert "core" not in PUBLISHED_TIER_NAMES
    assert not (DEV_BASE_ROOT / "core" / "Dockerfile").exists()
    assert "dev-base-core" not in DOCKERFILE.read_text(encoding="utf-8")


def test_language_targets_are_direct_independent_ubuntu_descendants() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8")
    for tier in LANGUAGE_TIERS:
        spec = TIER_BY_NAME[tier]
        assert spec.base_tier is None
        assert spec.dockerfile == DOCKERFILE
        assert spec.shared_context is True
        assert re.search(
            rf"^FROM ubuntu:\S+ AS {re.escape(spec.stage)}$", text, re.MULTILINE
        )
        stage = _stage_text(text, spec.stage)
        assert "ARG BASE_IMAGE" not in stage
        assert "FROM ${BASE_IMAGE}" not in stage


def test_language_targets_share_source_not_runtime_parentage() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8")
    assert text.count(
        "source=install-common.sh,target=/tmp/install-common.sh"
    ) == len(LANGUAGE_TIERS)
    assert (DEV_BASE_ROOT / "install-common.sh").is_file()
    assert text.count('ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]') == len(
        LANGUAGE_TIERS
    )
    assert text.count('CMD ["bash"]') == len(LANGUAGE_TIERS)


def test_version_defaults_have_one_owning_source() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8")
    defaults = re.findall(r"^ARG ([A-Z0-9_]+)=\S+", text, flags=re.MULTILINE)
    assert defaults
    assert len(defaults) == len(set(defaults))


def test_common_installer_uses_container_build_hygiene() -> None:
    dockerfile = DOCKERFILE.read_text(encoding="utf-8")
    installer = (DEV_BASE_ROOT / "install-common.sh").read_text(encoding="utf-8")
    dockerignore = (DEV_BASE_ROOT / ".dockerignore").read_text(encoding="utf-8")

    assert dockerfile.startswith("# syntax=docker/dockerfile:1")
    assert 'SHELL ["/bin/bash", "-o", "pipefail", "-c"]' in dockerfile
    assert "--no-install-recommends" in installer
    assert "rm -rf /var/lib/apt/lists/*" in installer
    assert "--retry 5 --retry-all-errors" in installer
    assert "org.opencontainers.image.source" in dockerfile
    assert dockerignore.startswith("**\n")
    assert "!install-common.sh" in dockerignore


def test_architecture_metadata_covers_every_runtime_tool_consumer() -> None:
    dockerfiles = (
        DOCKERFILE.read_text(encoding="utf-8")
        + FULL_DOCKERFILE.read_text(encoding="utf-8")
    )
    installer = (DEV_BASE_ROOT / "install-common.sh").read_text(encoding="utf-8")

    consumed = set(re.findall(r"\$\{([A-Z0-9_]+_ARCH)\}", dockerfiles))
    persisted = set(re.findall(r'"([A-Z0-9_]+_ARCH)=\$\{', installer))
    assert consumed <= persisted
    assert "/tmp/arch.env" not in dockerfiles


def test_every_language_target_runs_ward_doctor_after_common_setup() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8")
    for tier in LANGUAGE_TIERS:
        stage = _stage_text(text, TIER_BY_NAME[tier].stage)
        assert stage.index("install-common.sh /tmp/substrate-repos.txt") < stage.index(
            "ARG WARD_CONFIG_REF_COMMIT"
        )
        assert stage.index("ARG WARD_CONFIG_REF_COMMIT") < stage.index(
            "CLIGUARD_NO_SANDBOX=1 ward doctor"
        )


def test_full_remains_the_composed_default_surface() -> None:
    spec = TIER_BY_NAME["full"]
    assert spec.dockerfile == FULL_DOCKERFILE
    assert spec.base_tier == "lang-rust"
    assert spec.graft_tiers == ("lang-go", "lang-dotnet", "lang-python")

    text = FULL_DOCKERFILE.read_text(encoding="utf-8")
    assert text.startswith("# syntax=docker/dockerfile:1")
    assert "FROM ${BASE_IMAGE} AS dev-base-full" in text
    assert "COPY --from=dev-base-lang-go-graft /usr/local/go /usr/local/go" in text
    assert (
        "COPY --from=dev-base-lang-dotnet-graft /usr/local/dotnet /usr/local/dotnet"
        in text
    )
    assert (
        "COPY --from=dev-base-lang-python-graft /opt/uv/tools/pipenv "
        "/opt/uv/tools/pipenv" in text
    )


def test_publish_plan_models_parallel_languages_and_full_fan_in() -> None:
    tag = "candidate"
    plan = publish_plan(REGISTRY_BASE, tag)
    language_entries = [entry for entry in plan if entry["tier"] in LANGUAGE_TIERS]
    full = plan[-1]

    assert [entry["tier"] for entry in plan] == list(PUBLISHED_TIER_NAMES)
    assert all(entry["base_image"].startswith("ubuntu:") for entry in language_entries)
    assert all(entry["dockerfile"] == "docker/dev-base/Dockerfile" for entry in language_entries)
    assert all(entry["context_dir"] == "docker/dev-base" for entry in language_entries)
    assert full["tier"] == "full"
    assert full["base_image"] == f"{REGISTRY_BASE}:lang-rust-{tag}"
    assert full["graft_images"] == {
        "LANG_GO_IMAGE": f"{REGISTRY_BASE}:lang-go-{tag}",
        "LANG_DOTNET_IMAGE": f"{REGISTRY_BASE}:lang-dotnet-{tag}",
        "LANG_PYTHON_IMAGE": f"{REGISTRY_BASE}:lang-python-{tag}",
    }


def test_publish_plan_keeps_plain_full_tags_and_prefixed_language_tags() -> None:
    plan = publish_plan(REGISTRY_BASE, "candidate", ("release", "latest", "release"))
    refs = {entry["tier"]: entry for entry in plan}

    assert refs["full"]["image"] == f"{REGISTRY_BASE}:candidate"
    assert refs["full"]["alias_images"] == [
        f"{REGISTRY_BASE}:release",
        f"{REGISTRY_BASE}:latest",
    ]
    for tier in LANGUAGE_TIERS:
        assert refs[tier]["image"] == f"{REGISTRY_BASE}:{tier}-candidate"
        assert refs[tier]["cache_image"] == f"{REGISTRY_BASE}:{tier}-buildcache"


def test_language_build_targets_the_requested_stage_without_base_image(
    monkeypatch,
) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: False)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_inspect_manifest", lambda _ref: None)
    monkeypatch.setattr(script, "_probe_cache_write", lambda _ref: True)
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "commit")

    script._build_plan(
        REGISTRY_BASE,
        "candidate",
        True,
        "linux/amd64,linux/arm64",
        only_tier="lang-go",
    )

    build = next(cmd for cmd in commands if "--push" in cmd)
    build_args = [build[index + 1] for index, arg in enumerate(build) if arg == "--build-arg"]
    assert "--target" in build
    assert build[build.index("--target") + 1] == "dev-base-lang-go"
    assert "aos-cli=aos" in build
    assert "WARD_CONFIG_REF_COMMIT=commit" in build_args
    assert not any(arg.startswith("BASE_IMAGE=") for arg in build_args)


def test_local_language_build_sets_the_host_architecture(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_host_targetarch", lambda: "test-arch")
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "commit")

    script._build_plan(
        "agentic-os",
        "local",
        False,
        None,
        only_tier="lang-node",
    )

    build = commands[0]
    assert "TARGETARCH=test-arch" in build
    assert build[build.index("--target") + 1] == "dev-base-lang-node"


def test_full_build_consumes_only_published_language_refs(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: False)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_inspect_manifest", lambda _ref: None)
    monkeypatch.setattr(script, "_probe_cache_write", lambda _ref: True)
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "commit")

    script._build_plan(
        REGISTRY_BASE,
        "candidate",
        True,
        "linux/amd64,linux/arm64",
        only_tier="full",
    )

    build = next(cmd for cmd in commands if "--push" in cmd)
    build_args = [build[index + 1] for index, arg in enumerate(build) if arg == "--build-arg"]
    assert f"BASE_IMAGE={REGISTRY_BASE}:lang-rust-candidate" in build_args
    assert f"LANG_GO_IMAGE={REGISTRY_BASE}:lang-go-candidate" in build_args
    assert f"LANG_DOTNET_IMAGE={REGISTRY_BASE}:lang-dotnet-candidate" in build_args
    assert f"LANG_PYTHON_IMAGE={REGISTRY_BASE}:lang-python-candidate" in build_args
    assert "aos-cli=aos" not in build


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


def test_manifest_inspect_retries_and_reports_the_digest(
    monkeypatch, tmp_path
) -> None:
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

    monkeypatch.setattr(
        script.subprocess,
        "run",
        lambda *args, **kwargs: next(probes),
    )
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
        tier="lang-node",
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


def test_tier_tag_has_no_core_special_case() -> None:
    assert tier_tag("full", "candidate") == "candidate"
    assert tier_tag("lang-node", "candidate") == "lang-node-candidate"
