"""Tests for the dev-base image contract."""
from __future__ import annotations

import argparse
import importlib.util
import re
from pathlib import Path

from agentic_os.dev_base import (
    DEV_BASE_ROOT,
    REGISTRY_BASE,
    PUBLISHED_TIER_NAMES,
    TIER_SPECS,
    graft_build_arg,
    publish_plan,
    tier_tag,
)

REPO_ROOT = Path(__file__).resolve().parent.parent
SCRIPT = REPO_ROOT / "scripts" / "dev-base-build.py"


def _tier_path(tier: str) -> Path:
    return DEV_BASE_ROOT / tier / "Dockerfile"


def _load_script():
    spec = importlib.util.spec_from_file_location("dev_base_build", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_dev_base_tier_directories_exist() -> None:
    for tier in PUBLISHED_TIER_NAMES:
        assert _tier_path(tier).is_file()


def test_retired_non_language_tiers_are_absent() -> None:
    assert not _tier_path("ops").exists()
    assert not _tier_path("agent").exists()


def test_dev_base_plan_is_derived_from_tier_folder_names() -> None:
    tag = "v0.242.0"
    plan = publish_plan(REGISTRY_BASE, tag)

    assert [entry["tier"] for entry in plan] == list(PUBLISHED_TIER_NAMES)
    assert [entry["image"] for entry in plan] == [
        f"{REGISTRY_BASE}:{tier}-{tag}"
        for tier in PUBLISHED_TIER_NAMES
        if tier != "full"
    ] + [f"{REGISTRY_BASE}:{tag}"]
    assert [entry["stage"] for entry in plan] == [
        f"dev-base-{tier}" for tier in PUBLISHED_TIER_NAMES
    ]
    assert [entry["dockerfile"] for entry in plan] == [
        f"docker/dev-base/{tier}/Dockerfile" for tier in PUBLISHED_TIER_NAMES
    ]
    assert [entry["context_dir"] for entry in plan] == [
        "docker/dev-base",
        "docker/dev-base/lang-node",
        "docker/dev-base/lang-go",
        "docker/dev-base/lang-dotnet",
        "docker/dev-base/lang-rust",
        "docker/dev-base/lang-python",
        "docker/dev-base/full",
    ]
    # Core owns the common agent surface. Every language tier is its direct,
    # parallel child, while full fans the standalone toolchains back together.
    assert [entry["base_image"] for entry in plan] == [
        "ubuntu:24.04",
        *[f"{REGISTRY_BASE}:core-{tag}"] * 6,
    ]
    assert plan[-1]["tier"] == "full"
    assert "alias_image" not in plan[0]


def test_dev_base_plan_grafts_toolchains_into_the_composed_tiers() -> None:
    tag = "v0.242.0"
    plan = publish_plan(REGISTRY_BASE, tag)
    grafts = {entry["tier"]: entry["graft_images"] for entry in plan}

    assert grafts["full"] == {
        "LANG_GO_IMAGE": f"{REGISTRY_BASE}:lang-go-{tag}",
        "LANG_DOTNET_IMAGE": f"{REGISTRY_BASE}:lang-dotnet-{tag}",
        "LANG_RUST_IMAGE": f"{REGISTRY_BASE}:lang-rust-{tag}",
        "LANG_PYTHON_IMAGE": f"{REGISTRY_BASE}:lang-python-{tag}",
    }
    for tier in (
        "core",
        "lang-node",
        "lang-go",
        "lang-dotnet",
        "lang-rust",
        "lang-python",
    ):
        assert grafts[tier] == {}


def test_tier_specs_stay_in_topological_order() -> None:
    seen: set[str] = set()
    for spec in TIER_SPECS:
        if spec.base_tier is not None:
            assert spec.base_tier in seen, (
                f"{spec.tier}: base {spec.base_tier} not built yet"
            )
        for graft in spec.graft_tiers:
            assert graft in seen, f"{spec.tier}: graft {graft} not built yet"
        seen.add(spec.tier)


def test_dev_base_plan_carries_per_tier_cache_and_alias_refs() -> None:
    plan = publish_plan(REGISTRY_BASE, "v0.242.0", ("release", "latest", "release", ""))

    assert [entry["cache_image"] for entry in plan] == [
        f"{REGISTRY_BASE}:{tier_tag(tier, 'buildcache')}"
        for tier in PUBLISHED_TIER_NAMES
    ]
    assert [entry["alias_image"] for entry in plan] == [
        f"{REGISTRY_BASE}:{tier_tag(tier, 'release')}" for tier in PUBLISHED_TIER_NAMES
    ]
    assert [entry["alias_images"][1] for entry in plan] == [
        f"{REGISTRY_BASE}:{tier_tag(tier, 'latest')}" for tier in PUBLISHED_TIER_NAMES
    ]
    assert plan[-1]["cache_image"] == f"{REGISTRY_BASE}:buildcache"
    assert plan[-1]["alias_image"] == f"{REGISTRY_BASE}:release"
    assert plan[-1]["alias_images"] == [
        f"{REGISTRY_BASE}:release",
        f"{REGISTRY_BASE}:latest",
    ]


def test_core_tier_keeps_the_hidden_ward_builder_stage() -> None:
    text = _tier_path("core").read_text()
    assert "AS dev-base-ward-builder" in text
    assert (
        'COPY --from=dev-base-ward-builder /usr/local/bin/ward /usr/local/bin/ward'
        in text
    )
    assert "ARG WARD_CONFIG_REF_COMMIT" in text
    # The forge host is owned by the ops guardfile - derive it, never restate it.
    ops_guardfile = (REPO_ROOT / ".ward" / "guardfile.forgejo.kdl").read_text()
    host = re.search(r'base-url "([^/"]+)', ops_guardfile).group(1)
    assert (
        f"WARD_CONFIG_REF={host}/coilyco-flight-deck/agentic-os"
        "@${WARD_CONFIG_REF_COMMIT}//.ward"
    ) in text


def test_core_tier_stamps_ward_config_after_cacheable_tool_layers() -> None:
    text = _tier_path("core").read_text()
    builder, core = text.split("FROM ubuntu:24.04 AS dev-base-core", maxsplit=1)
    ward_ref = "ENV WARD_CONFIG_REF="

    assert "WARD_CONFIG_REF_COMMIT" not in builder
    assert ward_ref not in builder
    assert core.index("uv python install") < core.index(ward_ref)
    assert core.index("COPY --from=dev-base-ward-builder") < core.index(
        "ARG WARD_CONFIG_REF_COMMIT"
    )
    assert core.index("ARG WARD_CONFIG_REF_COMMIT") < core.index(ward_ref)
    assert core.index(ward_ref) < core.index("ward doctor")


def test_core_tier_runs_ward_doctor_after_installing_ward() -> None:
    text = _tier_path("core").read_text()
    assert (
        text.index(
            'COPY --from=dev-base-ward-builder /usr/local/bin/ward /usr/local/bin/ward'
        )
        < text.index("ward doctor")
    )
    assert "CLIGUARD_NO_SANDBOX=1 ward doctor" in text


def test_tier_tag_keeps_full_plain_and_prefixes_variants() -> None:
    assert tier_tag("full", "v0.243.0") == "v0.243.0"
    assert tier_tag("full", "release") == "release"
    assert tier_tag("core", "v0.243.0") == "core-v0.243.0"
    assert tier_tag("lang-node", "release") == "lang-node-release"


def test_tier_files_build_from_their_declared_base_image() -> None:
    for tier in (
        "lang-node",
        "lang-go",
        "lang-dotnet",
        "lang-rust",
        "lang-python",
        "full",
    ):
        text = _tier_path(tier).read_text()
        assert f"FROM ${{BASE_IMAGE}} AS dev-base-{tier}" in text
        assert "ARG BASE_IMAGE" in text


def test_composed_tier_files_graft_their_declared_toolchain_tiers() -> None:
    # Each declared graft needs an ARG-driven FROM stage plus a COPY --from of
    # it, so the plan's graft_images build-args actually land in the image.
    for spec in TIER_SPECS:
        text = spec.dockerfile.read_text()
        for graft in spec.graft_tiers:
            arg = graft_build_arg(graft)
            assert f"ARG {arg}" in text, f"{spec.tier}: missing ARG {arg}"
            assert f"FROM ${{{arg}}} AS dev-base-{graft}-graft" in text
            assert f"COPY --from=dev-base-{graft}-graft " in text
    full_text = _tier_path("full").read_text()
    assert "COPY --from=dev-base-lang-go-graft /usr/local/go /usr/local/go" in full_text
    assert (
        "COPY --from=dev-base-lang-dotnet-graft /usr/local/dotnet /usr/local/dotnet"
        in full_text
    )
    assert (
        "COPY --from=dev-base-lang-python-graft /opt/uv/tools/pipenv /opt/uv/tools/pipenv"
        in full_text
    )


def test_node_prefix_is_on_the_core_path() -> None:
    assert "/usr/local/node/bin" in _tier_path("core").read_text()


def test_core_copies_the_stable_assets_from_the_root_context() -> None:
    text = _tier_path("core").read_text()
    assert "COPY agent-name.sh /opt/agentic-os/agent-name.sh" in text
    assert "COPY statusline.sh /opt/agentic-os/statusline.sh" in text
    assert "COPY statusline.d/ /opt/agentic-os/statusline.d/" in text
    assert (
        "COPY ward-shell-entrypoint.sh /opt/agentic-os/ward-shell-entrypoint.sh"
        in text
    )
    assert (
        "COPY claude-managed-settings.json /etc/claude-code/managed-settings.json"
        in text
    )
    assert "COPY substrate-image-repos.txt /tmp/substrate-image-repos.txt" in text
    assert 'ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]' in text


def test_core_owns_the_shared_harness_and_operational_tools() -> None:
    text = _tier_path("core").read_text()
    for version_arg in (
        "CLAUDE_VERSION",
        "MCPORTER_VERSION",
        "OPENCODE_VERSION",
        "CODEX_VERSION",
        "GOOSE_VERSION",
        "AWSCLI_VERSION",
        "GH_VERSION",
        "DOCKER_VERSION",
        "HELM_VERSION",
        "KUBECTL_VERSION",
        "YQ_VERSION",
        "TAILSCALE_VERSION",
    ):
        assert f"ARG {version_arg}=" in text
    for command in (
        "claude --version",
        "mcporter --help",
        "opencode --version",
        "codex --version",
        "goose --version",
        "aws --version",
        "gh --version",
        "helm version",
        "kubectl version",
        "yq --version",
        "docker --version",
        "tailscale version",
        "tailscaled --version",
    ):
        assert command in text


def test_python_specialist_exposes_the_expected_python_tooling() -> None:
    text = _tier_path("lang-python").read_text()
    assert "ln -sf /usr/bin/python3 /usr/local/bin/python" in text
    assert "ln -sf /usr/bin/pip3 /usr/local/bin/pip" in text
    assert "uv tool install pipenv" in text
    for command in ("python --version", "pip --version", "pipenv --version", "uv --version"):
        assert command in text


def test_full_tier_remains_the_default_surface() -> None:
    text = _tier_path("full").read_text()
    assert "WORKDIR /workspace" in text
    assert 'CMD ["bash"]' in text


def test_old_manifest_is_gone() -> None:
    assert not (DEV_BASE_ROOT / "Dockerfile").exists()
    assert not (DEV_BASE_ROOT / "ci-image-manifest.json").exists()


def test_pushed_build_uses_release_tagless_cache_ref(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: False)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(
        script,
        "_inspect_manifest",
        lambda ref: "sha256:cache" if "buildcache" in ref else None,
    )
    monkeypatch.setattr(script, "_probe_cache_write", lambda ref: True)
    monkeypatch.setattr(script, "_host_targetarch", lambda: "amd64")
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "abc123")

    script._build_plan(REGISTRY_BASE, "v0.243.0", True, "linux/amd64,linux/arm64")

    cache_refs = [
        arg
        for cmd in commands
        for arg in cmd
        if arg.startswith("type=registry,ref=")
    ]
    assert cache_refs
    assert all("v0.243.0" not in ref for ref in cache_refs)
    assert f"type=registry,ref={REGISTRY_BASE}:core-buildcache" in cache_refs
    assert (
        f"type=registry,ref={REGISTRY_BASE}:buildcache,mode=max,ignore-error=true"
        in cache_refs
    )
    assert any(
        "WARD_CONFIG_REF_COMMIT=abc123" in arg
        for cmd in commands
        for arg in cmd
    )


def test_dev_base_plan_uses_tier_local_build_contexts() -> None:
    plan = publish_plan(REGISTRY_BASE, "v0.243.0")

    assert plan[0]["context_dir"] == "docker/dev-base"
    assert plan[1]["context_dir"] == "docker/dev-base/lang-node"
    assert plan[-1]["context_dir"] == "docker/dev-base/full"


def test_core_push_emits_a_release_context_breadcrumb(
    monkeypatch, capsys, tmp_path
) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: False)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(
        script,
        "_inspect_manifest",
        lambda ref: "sha256:cache" if "buildcache" in ref else None,
    )
    monkeypatch.setattr(script, "_probe_cache_write", lambda ref: True)
    monkeypatch.setattr(script, "_host_targetarch", lambda: "amd64")
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "abc123")
    summary_file = tmp_path / "core-publish-summary.md"
    monkeypatch.setenv("GITHUB_STEP_SUMMARY", str(summary_file))

    script._build_plan(
        REGISTRY_BASE,
        "v0.243.0",
        True,
        "linux/amd64,linux/arm64",
        only_tier="core",
    )

    captured = capsys.readouterr().out
    assert "::group::core publish context" in captured
    assert "tier=core" in captured
    assert f"image={REGISTRY_BASE}:core-v0.243.0" in captured
    assert f"cache={REGISTRY_BASE}:core-buildcache" in captured
    assert "ward_config_ref_commit=abc123" in captured
    assert "context=docker/dev-base" in captured
    assert "docker buildx build --progress=plain --push --platform linux/amd64,linux/arm64" in captured
    assert "cache_source_provenance=sha256:cache" in captured
    assert commands
    summary = summary_file.read_text(encoding="utf-8")
    assert "### core publish context" in summary
    assert "- tier: core" in summary
    assert f"- image: {REGISTRY_BASE}:core-v0.243.0" in summary
    assert f"- cache: {REGISTRY_BASE}:core-buildcache" in summary
    assert "- base: ubuntu:24.04" in summary
    assert "- context: docker/dev-base" in summary
    assert "- ward_config_ref_commit: abc123" in summary
    assert "command: docker buildx build --progress=plain --push --platform linux/amd64,linux/arm64" in summary
    assert "### core cache plan" in summary
    assert f"- cache key: {REGISTRY_BASE}:core-buildcache" in summary
    assert f"- cache source: type=registry,ref={REGISTRY_BASE}:core-buildcache" in summary
    assert (
        f"- cache destination: type=registry,ref={REGISTRY_BASE}:core-buildcache,mode=max,ignore-error=true"
        in summary
    )
    assert "- cache source provenance: sha256:cache" in summary
    assert "### core cache result" in summary
    assert "- cache write: verified" in summary


def test_pushed_build_skips_an_existing_checkpoint(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []
    probed: list[str] = []

    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: True)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_inspect_manifest", lambda ref: "sha256:cache")
    monkeypatch.setattr(
        script, "_probe_cache_write", lambda ref: probed.append(ref) or True
    )
    monkeypatch.setattr(script, "_host_targetarch", lambda: "amd64")
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "abc123")

    script._build_plan(
        REGISTRY_BASE,
        "v0.243.0",
        True,
        "linux/amd64,linux/arm64",
        ("release",),
        "full",
    )

    assert commands == []
    assert probed == []


def test_pushed_build_tags_every_tier_with_the_requested_aliases(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: False)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(
        script,
        "_inspect_manifest",
        lambda ref: "sha256:cache" if "buildcache" in ref else None,
    )
    monkeypatch.setattr(script, "_probe_cache_write", lambda ref: True)
    monkeypatch.setattr(script, "_host_targetarch", lambda: "amd64")
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "abc123")

    script._build_plan(
        REGISTRY_BASE,
        "v0.243.0",
        True,
        "linux/amd64,linux/arm64",
        ("release", "latest"),
    )

    build_cmds = [cmd for cmd in commands if "--push" in cmd]
    assert len(build_cmds) == len(PUBLISHED_TIER_NAMES)
    for tier, cmd in zip(PUBLISHED_TIER_NAMES, build_cmds):
        tags = [cmd[i + 1] for i, arg in enumerate(cmd) if arg == "-t"]
        assert tags == [
            f"{REGISTRY_BASE}:{tier_tag(tier, 'v0.243.0')}",
            f"{REGISTRY_BASE}:{tier_tag(tier, 'release')}",
            f"{REGISTRY_BASE}:{tier_tag(tier, 'latest')}",
        ]
    # The push path also verifies the alias manifest per tier, not just the tag.
    inspect_cmds = [
        cmd
        for cmd in commands
        if cmd[:4] == ["docker", "buildx", "imagetools", "inspect"]
    ]
    assert f"{REGISTRY_BASE}:release" in [cmd[-1] for cmd in inspect_cmds]
    assert f"{REGISTRY_BASE}:latest" in [cmd[-1] for cmd in inspect_cmds]


def test_promote_plan_retags_a_draft_to_release_aliases(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    class _Probe:
        returncode = 0
        stderr = ""

    monkeypatch.setattr(script.subprocess, "run", lambda *a, **k: _Probe())
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))

    script._promote_plan(
        REGISTRY_BASE, "draft-abc123", "v0.244.0", ("release", "latest"), "full"
    )

    create_cmds = [
        cmd
        for cmd in commands
        if cmd[:4] == ["docker", "buildx", "imagetools", "create"]
    ]
    assert create_cmds == [
        [
            "docker",
            "buildx",
            "imagetools",
            "create",
            "-t",
            f"{REGISTRY_BASE}:v0.244.0",
            "-t",
            f"{REGISTRY_BASE}:release",
            "-t",
            f"{REGISTRY_BASE}:latest",
            f"{REGISTRY_BASE}:draft-abc123",
        ]
    ]
    inspect_refs = [
        cmd[-1]
        for cmd in commands
        if cmd[:4] == ["docker", "buildx", "imagetools", "inspect"]
    ]
    assert inspect_refs == [
        f"{REGISTRY_BASE}:v0.244.0",
        f"{REGISTRY_BASE}:release",
        f"{REGISTRY_BASE}:latest",
    ]


def test_promote_plan_waits_for_the_source_image_before_retagging(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []
    sleeps: list[int] = []

    class _Probe:
        def __init__(self, returncode: int, stderr: str) -> None:
            self.returncode = returncode
            self.stderr = stderr

    probes = iter(
        [
            _Probe(1, "manifest unknown"),
            _Probe(1, "manifest unknown"),
            _Probe(0, ""),
        ]
    )

    def fake_run(cmd, check=False, capture_output=False, text=False):  # noqa: ANN001
        assert cmd[:4] == ["docker", "buildx", "imagetools", "inspect"]
        return next(probes)

    monkeypatch.setattr(script.subprocess, "run", fake_run)
    monkeypatch.setattr(script.time, "sleep", lambda seconds: sleeps.append(seconds))
    monkeypatch.setattr(script, "_target_matches_source", lambda *_args: False)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))

    script._promote_plan(
        REGISTRY_BASE, "draft-abc123", "v0.244.0", ("release", "latest"), "full"
    )

    assert sleeps == [5, 10]
    assert commands[0] == [
        "docker",
        "buildx",
        "imagetools",
        "create",
        "-t",
        f"{REGISTRY_BASE}:v0.244.0",
        "-t",
        f"{REGISTRY_BASE}:release",
        "-t",
        f"{REGISTRY_BASE}:latest",
        f"{REGISTRY_BASE}:draft-abc123",
    ]


def test_manifest_inspect_retries_and_writes_a_success_note(
    monkeypatch, capsys, tmp_path
) -> None:
    script = _load_script()
    sleeps: list[int] = []

    class _Probe:
        def __init__(self, returncode: int, stdout: str, stderr: str) -> None:
            self.returncode = returncode
            self.stdout = stdout
            self.stderr = stderr

    probes = iter(
        [
            _Probe(1, "", "manifest unknown"),
            _Probe(0, "Digest: sha256:abc123", ""),
        ]
    )

    def fake_run(cmd, check=False, capture_output=False, text=False):  # noqa: ANN001
        assert cmd[:4] == ["docker", "buildx", "imagetools", "inspect"]
        return next(probes)

    monkeypatch.setattr(script.subprocess, "run", fake_run)
    monkeypatch.setattr(script.time, "sleep", lambda seconds: sleeps.append(seconds))
    summary_file = tmp_path / "summary.md"
    monkeypatch.setenv("GITHUB_STEP_SUMMARY", str(summary_file))

    assert script._inspect_manifest(
        f"{REGISTRY_BASE}:core-v0.243.0"
    ) == "sha256:abc123"

    err = capsys.readouterr().err
    assert "retrying inspect manifest" in err
    assert "succeeded after attempt 2/3" in err
    assert sleeps == [1]
    summary = summary_file.read_text(encoding="utf-8")
    assert "- inspect manifest" in summary
    assert "succeeded after attempt 2/3" in summary


def test_promote_plan_skips_an_existing_checkpoint(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    monkeypatch.setattr(script, "_target_matches_source", lambda *_args: True)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))

    script._promote_plan(
        REGISTRY_BASE, "draft-abc123", "v0.244.0", ("release", "latest"), "full"
    )

    assert commands == []


def test_cmd_check_reports_the_requested_checkpoint_state(monkeypatch) -> None:
    script = _load_script()

    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: True)
    monkeypatch.setattr(script, "_target_matches_source", lambda *_args: True)

    build_args = argparse.Namespace(
        registry=REGISTRY_BASE,
        tag="v0.243.0",
        alias=["release"],
        mode="build",
        tier="full",
        source_tag="",
    )
    promote_args = argparse.Namespace(
        registry=REGISTRY_BASE,
        tag="v0.244.0",
        alias=["release", "latest"],
        mode="promote",
        tier="full",
        source_tag="draft-abc123",
    )

    assert script._cmd_check(build_args) == 0
    assert script._cmd_check(promote_args) == 0

    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: False)
    monkeypatch.setattr(script, "_target_matches_source", lambda *_args: False)
    assert script._cmd_check(build_args) == 1
    assert script._cmd_check(promote_args) == 1


def test_single_tier_push_builds_only_that_tier_with_graft_args(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []
    probed: list[str] = []

    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: False)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(
        script,
        "_inspect_manifest",
        lambda ref: "sha256:cache" if "buildcache" in ref else None,
    )
    monkeypatch.setattr(
        script,
        "_probe_cache_write",
        lambda ref: probed.append(ref) or True,
    )
    monkeypatch.setattr(script, "_host_targetarch", lambda: "amd64")
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "abc123")

    script._build_plan(
        REGISTRY_BASE, "v0.243.0", True, "linux/amd64,linux/arm64", "release", "full"
    )

    build_cmds = [cmd for cmd in commands if "--push" in cmd]
    assert len(build_cmds) == 1
    cmd = build_cmds[0]
    build_args = [cmd[i + 1] for i, arg in enumerate(cmd) if arg == "--build-arg"]
    assert f"BASE_IMAGE={REGISTRY_BASE}:core-v0.243.0" in build_args
    assert f"LANG_GO_IMAGE={REGISTRY_BASE}:lang-go-v0.243.0" in build_args
    assert f"LANG_DOTNET_IMAGE={REGISTRY_BASE}:lang-dotnet-v0.243.0" in build_args
    assert f"LANG_RUST_IMAGE={REGISTRY_BASE}:lang-rust-v0.243.0" in build_args
    assert f"LANG_PYTHON_IMAGE={REGISTRY_BASE}:lang-python-v0.243.0" in build_args
    assert probed == [f"{REGISTRY_BASE}:buildcache"]


def test_single_tier_build_rejects_an_unknown_tier(monkeypatch) -> None:
    script = _load_script()
    monkeypatch.setattr(script, "_run", lambda cmd: None)
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "abc123")

    try:
        script._build_plan(REGISTRY_BASE, "v0.243.0", True, None, None, "nope")
    except SystemExit as exc:
        assert "unknown tier" in str(exc)
    else:
        raise AssertionError("unknown tier did not raise")


def test_cache_probe_warns_loudly_when_the_cache_write_is_missing(
    monkeypatch, capsys
) -> None:
    script = _load_script()

    class _Probe:
        returncode = 1
        stderr = "manifest unknown"

    monkeypatch.setattr(script.subprocess, "run", lambda *a, **k: _Probe())
    monkeypatch.setattr(script.time, "sleep", lambda _seconds: None)
    assert script._probe_cache_write(f"{REGISTRY_BASE}:buildcache") is False
    err = capsys.readouterr().err
    assert "::warning::" in err
    assert "starts cold" in err


def test_local_build_does_not_tag_an_alias(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_host_targetarch", lambda: "amd64")
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "abc123")

    script._build_plan("agentic-os", "dev-base-local", False, None)

    for cmd in commands:
        tags = [cmd[i + 1] for i, arg in enumerate(cmd) if arg == "-t"]
        assert tags == [tag for tag in tags if tag.endswith("dev-base-local")]
