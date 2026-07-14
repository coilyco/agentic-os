"""Tests for the dev-base image contract."""
from __future__ import annotations

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


def test_dev_base_plan_is_derived_from_tier_folder_names() -> None:
    tag = "v0.242.0"
    plan = publish_plan(REGISTRY_BASE, tag)

    assert [entry["tier"] for entry in plan] == list(PUBLISHED_TIER_NAMES)
    assert [entry["image"] for entry in plan] == [
        f"{REGISTRY_BASE}:{tier}-{tag}" for tier in PUBLISHED_TIER_NAMES if tier != "full"
    ] + [f"{REGISTRY_BASE}:{tag}"]
    assert [entry["stage"] for entry in plan] == [
        f"dev-base-{tier}" for tier in PUBLISHED_TIER_NAMES
    ]
    assert [entry["dockerfile"] for entry in plan] == [
        f"docker/dev-base/{tier}/Dockerfile" for tier in PUBLISHED_TIER_NAMES
    ]
    # The fan-out DAG (aos#491): the lang tiers and ops are siblings on core,
    # agent composes ops, full fans in on agent.
    assert [entry["base_image"] for entry in plan] == [
        "ubuntu:24.04",
        f"{REGISTRY_BASE}:core-{tag}",
        f"{REGISTRY_BASE}:core-{tag}",
        f"{REGISTRY_BASE}:core-{tag}",
        f"{REGISTRY_BASE}:core-{tag}",
        f"{REGISTRY_BASE}:ops-{tag}",
        f"{REGISTRY_BASE}:agent-{tag}",
    ]
    assert plan[-1]["tier"] == "full"
    assert "alias_image" not in plan[0]


def test_dev_base_plan_grafts_toolchains_into_the_composed_tiers() -> None:
    tag = "v0.242.0"
    plan = publish_plan(REGISTRY_BASE, tag)
    grafts = {entry["tier"]: entry["graft_images"] for entry in plan}

    assert grafts["agent"] == {"LANG_NODE_IMAGE": f"{REGISTRY_BASE}:lang-node-{tag}"}
    assert grafts["full"] == {
        "LANG_GO_IMAGE": f"{REGISTRY_BASE}:lang-go-{tag}",
        "LANG_DOTNET_IMAGE": f"{REGISTRY_BASE}:lang-dotnet-{tag}",
    }
    for tier in ("core", "lang-node", "lang-go", "lang-dotnet", "ops"):
        assert grafts[tier] == {}


def test_tier_specs_stay_in_topological_order() -> None:
    seen: set[str] = set()
    for spec in TIER_SPECS:
        if spec.base_tier is not None:
            assert spec.base_tier in seen, f"{spec.tier}: base {spec.base_tier} not built yet"
        for graft in spec.graft_tiers:
            assert graft in seen, f"{spec.tier}: graft {graft} not built yet"
        seen.add(spec.tier)


def test_dev_base_plan_carries_per_tier_cache_and_alias_refs() -> None:
    plan = publish_plan(REGISTRY_BASE, "v0.242.0", ("release", "latest", "release", ""))

    assert [entry["cache_image"] for entry in plan] == [
        f"{REGISTRY_BASE}:{tier_tag(tier, 'buildcache')}" for tier in PUBLISHED_TIER_NAMES
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
    assert 'COPY --from=dev-base-ward-builder /usr/local/bin/ward /usr/local/bin/ward' in text
    assert "ARG WARD_CONFIG_REF_COMMIT" in text
    # The forge host is owned by the ops guardfile - derive it, never restate it.
    ops_guardfile = (REPO_ROOT / ".ward" / "guardfile.forgejo.kdl").read_text()
    host = re.search(r'base-url "([^/"]+)', ops_guardfile).group(1)
    assert (
        f"WARD_CONFIG_REF={host}/coilyco-flight-deck/agentic-os"
        "@${WARD_CONFIG_REF_COMMIT}//.ward"
    ) in text


def test_core_tier_runs_ward_doctor_after_installing_ward() -> None:
    text = _tier_path("core").read_text()
    assert (
        text.index('COPY --from=dev-base-ward-builder /usr/local/bin/ward /usr/local/bin/ward')
        < text.index("ward doctor")
    )
    assert "CLIGUARD_NO_SANDBOX=1 ward doctor" in text


def test_tier_tag_keeps_full_plain_and_prefixes_variants() -> None:
    assert tier_tag("full", "v0.243.0") == "v0.243.0"
    assert tier_tag("full", "release") == "release"
    assert tier_tag("core", "v0.243.0") == "core-v0.243.0"
    assert tier_tag("lang-node", "release") == "lang-node-release"


def test_tier_files_build_from_their_declared_base_image() -> None:
    for tier in ("lang-node", "lang-go", "lang-dotnet", "ops", "agent", "full"):
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
    agent_text = _tier_path("agent").read_text()
    full_text = _tier_path("full").read_text()
    assert "COPY --from=dev-base-lang-node-graft /usr/local/node /usr/local/node" in agent_text
    assert "COPY --from=dev-base-lang-go-graft /usr/local/go /usr/local/go" in full_text
    assert (
        "COPY --from=dev-base-lang-dotnet-graft /usr/local/dotnet /usr/local/dotnet" in full_text
    )


def test_node_graft_prefix_is_on_the_core_path() -> None:
    # Cross-tier wiring: core bakes the PATH entry so the node prefix grafted
    # into agent (see the graft test above) lands runnable.
    assert "/usr/local/node/bin" in _tier_path("core").read_text()


def test_agent_tier_still_copies_the_stable_assets_from_the_root_context() -> None:
    text = _tier_path("agent").read_text()
    assert "COPY agent-name.sh /opt/agentic-os/agent-name.sh" in text
    assert "COPY statusline.sh /opt/agentic-os/statusline.sh" in text
    assert "COPY statusline.d/ /opt/agentic-os/statusline.d/" in text
    assert "COPY ward-shell-entrypoint.sh /opt/agentic-os/ward-shell-entrypoint.sh" in text
    assert "COPY claude-managed-settings.json /etc/claude-code/managed-settings.json" in text
    assert "COPY substrate-image-repos.txt /tmp/substrate-image-repos.txt" in text
    assert 'ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]' in text


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

    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_probe_cache_write", lambda ref: None)
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
    assert f"type=registry,ref={REGISTRY_BASE}:buildcache,mode=max,ignore-error=true" in cache_refs
    assert any(
        "WARD_CONFIG_REF_COMMIT=abc123" in arg
        for cmd in commands
        for arg in cmd
    )


def test_pushed_build_tags_every_tier_with_the_requested_aliases(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_probe_cache_write", lambda ref: None)
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
    inspect_cmds = [cmd for cmd in commands if cmd[:4] == ["docker", "buildx", "imagetools", "inspect"]]
    assert f"{REGISTRY_BASE}:release" in [cmd[-1] for cmd in inspect_cmds]
    assert f"{REGISTRY_BASE}:latest" in [cmd[-1] for cmd in inspect_cmds]


def test_promote_plan_retags_a_draft_to_release_aliases(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []

    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))

    script._promote_plan(REGISTRY_BASE, "draft-abc123", "v0.244.0", ("release", "latest"), "full")

    create_cmds = [cmd for cmd in commands if cmd[:4] == ["docker", "buildx", "imagetools", "create"]]
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


def test_single_tier_push_builds_only_that_tier_with_graft_args(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []
    probed: list[str] = []

    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_probe_cache_write", lambda ref: probed.append(ref))
    monkeypatch.setattr(script, "_host_targetarch", lambda: "amd64")
    monkeypatch.setattr(script, "_ward_config_ref_commit", lambda: "abc123")

    script._build_plan(
        REGISTRY_BASE, "v0.243.0", True, "linux/amd64,linux/arm64", "release", "full"
    )

    build_cmds = [cmd for cmd in commands if "--push" in cmd]
    assert len(build_cmds) == 1
    cmd = build_cmds[0]
    build_args = [cmd[i + 1] for i, arg in enumerate(cmd) if arg == "--build-arg"]
    assert f"BASE_IMAGE={REGISTRY_BASE}:agent-v0.243.0" in build_args
    assert f"LANG_GO_IMAGE={REGISTRY_BASE}:lang-go-v0.243.0" in build_args
    assert f"LANG_DOTNET_IMAGE={REGISTRY_BASE}:lang-dotnet-v0.243.0" in build_args
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


def test_cache_probe_warns_loudly_when_the_cache_write_is_missing(monkeypatch, capsys) -> None:
    script = _load_script()

    class _Probe:
        returncode = 1
        stderr = "manifest unknown"

    monkeypatch.setattr(script.subprocess, "run", lambda *a, **k: _Probe())
    script._probe_cache_write(f"{REGISTRY_BASE}:buildcache")
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
