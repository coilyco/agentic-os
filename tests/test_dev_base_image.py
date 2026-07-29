"""Tests for the independent dev-base language-image contract."""

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
INSTALL_COMMON = DEV_BASE_ROOT / "install-common.sh"
VERIFY_COMMON = DEV_BASE_ROOT / "verify-common.sh"
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


def test_language_targets_are_direct_independent_ubuntu_descendants() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8")

    assert PUBLISHED_TIER_NAMES == (
        "lang-node",
        "lang-go",
        "lang-dotnet",
        "lang-rust",
        "lang-python",
        "full",
    )
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
    assert text.count(
        "source=verify-common.sh,target=/tmp/verify-common.sh"
    ) == len(LANGUAGE_TIERS)
    assert text.count('ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]') == len(
        LANGUAGE_TIERS
    )
    assert text.count('CMD ["bash"]') == len(LANGUAGE_TIERS)
    assert "specgen-linux-${TARGETARCH}" in text
    assert "sha256sum -c -" in text
    assert "COPY --from=aosguard-spec" in text
    assert "COPY --from=aosguard-python" in text
    assert "--skills-out /opt/agentic-os/aosguard-skill" in text
    assert (
        "COPY --from=dev-base-tool-builder /opt/agentic-os/aosguard-skill "
        "/opt/agentic-os/aosguard-skill"
    ) in text
    assert text.count(
        "COPY --from=dev-base-tool-builder /opt/agentic-os/aosguard-skill "
        "/opt/agentic-os/aosguard-skill"
    ) == len(LANGUAGE_TIERS)


def test_common_verification_covers_the_composed_runtime_surface() -> None:
    text = VERIFY_COMMON.read_text(encoding="utf-8")

    for command in (
        "aosguard --version",
        "agent-compose version",
        "agent-compose roster",
        "person.json",
        "python3 -m agentic_os.forgejo_actions_list --help",
        "ward --version",
        "WARD_DOCTOR_ALLOW_PLACEHOLDERS=1 ward doctor",
    ):
        assert command in text
    assert "test -s /opt/agentic-os/aosguard-skill/aosguard/SKILL.md" in text
    assert (
        "test -s /opt/agentic-os/aosguard-skill/aosguard/references/commands.yaml"
        in text
    )


def test_substrate_seed_parser_accepts_windows_line_endings() -> None:
    text = INSTALL_COMMON.read_text(encoding="utf-8")

    assert "ref=${ref%$'\\r'}" in text


def test_version_defaults_have_one_owning_source() -> None:
    text = (
        DOCKERFILE.read_text(encoding="utf-8")
        + FULL_DOCKERFILE.read_text(encoding="utf-8")
    )
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


def test_linuxbrew_is_absent_and_transient_npm_cache_is_removed() -> None:
    text = (
        DOCKERFILE.read_text(encoding="utf-8")
        + FULL_DOCKERFILE.read_text(encoding="utf-8")
        + INSTALL_COMMON.read_text(encoding="utf-8")
    )

    assert "linuxbrew" not in text.lower()
    assert "/home/linuxbrew" not in text
    assert "rm -rf /root/.npm" in text


def test_full_remains_the_composed_default_surface() -> None:
    spec = TIER_BY_NAME["full"]
    assert spec.dockerfile == FULL_DOCKERFILE
    assert spec.base_tier == "lang-rust"
    assert spec.graft_tiers == ("lang-go", "lang-dotnet", "lang-python")

    text = FULL_DOCKERFILE.read_text(encoding="utf-8")
    assert "FROM ${BASE_IMAGE} AS dev-base-full" in text
    assert (
        "PATH=/usr/local/go/bin:/usr/local/dotnet:/usr/local/node/bin:"
        "/usr/local/cargo/bin:" in text
    )
    assert "DOTNET_ROOT=/usr/local/dotnet" in text
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
    plan = publish_plan(REGISTRY_BASE, "candidate", ("release", "latest"))

    assert [entry["tier"] for entry in plan] == list(PUBLISHED_TIER_NAMES)
    for entry in plan[:-1]:
        assert entry["base_image"] == "ubuntu:24.04"
        assert entry["dockerfile"] == "docker/dev-base/Dockerfile"
        assert entry["context_dir"] == "docker/dev-base"
        assert entry["image"] == (
            f"{REGISTRY_BASE}:{entry['tier']}-candidate"
        )
    full = plan[-1]
    assert full["image"] == f"{REGISTRY_BASE}:candidate"
    assert full["base_image"] == f"{REGISTRY_BASE}:lang-rust-candidate"
    assert full["graft_images"] == {
        "LANG_GO_IMAGE": f"{REGISTRY_BASE}:lang-go-candidate",
        "LANG_DOTNET_IMAGE": f"{REGISTRY_BASE}:lang-dotnet-candidate",
        "LANG_PYTHON_IMAGE": f"{REGISTRY_BASE}:lang-python-candidate",
    }
    assert full["alias_images"] == [
        f"{REGISTRY_BASE}:release",
        f"{REGISTRY_BASE}:latest",
    ]


def test_language_tags_are_prefixed_and_full_tags_stay_plain() -> None:
    assert tier_tag("lang-node", "candidate") == "lang-node-candidate"
    assert tier_tag("full", "candidate") == "candidate"


def test_local_language_build_sets_architecture_and_named_contexts(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))
    monkeypatch.setattr(script, "_host_targetarch", lambda: "amd64")

    script._build_plan(
        REGISTRY_BASE,
        "candidate",
        False,
        None,
        only_tier="lang-go",
    )

    assert len(commands) == 1
    build = commands[0]
    assert build[:3] == ["docker", "build", "--build-arg"]
    assert "TARGETARCH=amd64" in build
    assert "aos-cli=aos" in build
    assert "aosguard-spec=.specgen" in build
    assert "aosguard-python=agentic_os" in build
    assert build[build.index("--target") + 1] == "dev-base-lang-go"
    assert Path(build[-1]).as_posix() == "docker/dev-base"


def test_full_build_consumes_only_language_image_refs(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))

    script._build_plan(
        REGISTRY_BASE,
        "candidate",
        False,
        None,
        only_tier="full",
    )

    build = commands[0]
    build_args = [
        build[index + 1] for index, value in enumerate(build) if value == "--build-arg"
    ]
    assert f"BASE_IMAGE={REGISTRY_BASE}:lang-rust-candidate" in build_args
    assert f"LANG_GO_IMAGE={REGISTRY_BASE}:lang-go-candidate" in build_args
    assert f"LANG_DOTNET_IMAGE={REGISTRY_BASE}:lang-dotnet-candidate" in build_args
    assert f"LANG_PYTHON_IMAGE={REGISTRY_BASE}:lang-python-candidate" in build_args
    assert "aos-cli=aos" not in build
    assert build[build.index("--target") + 1] == "dev-base-full"
    assert Path(build[-1]).as_posix() == "docker/dev-base/full"


def test_pushed_build_skips_an_existing_checkpoint(monkeypatch) -> None:
    script = _load_script()
    commands: list[list[str]] = []
    monkeypatch.setattr(script, "_has_target_checkpoint", lambda *_args: True)
    monkeypatch.setattr(script, "_run", lambda cmd: commands.append(cmd))

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


def test_manifest_inspect_treats_registry_404_as_immediate_miss(monkeypatch) -> None:
    script = _load_script()
    calls: list[dict[str, object]] = []
    sleeps: list[int] = []

    class Probe:
        returncode = 1
        stdout = ""
        stderr = "unexpected status from HEAD request: 404 Not Found"

    def run(*_args, **kwargs):
        calls.append(kwargs)
        return Probe()

    monkeypatch.setattr(script.subprocess, "run", run)
    monkeypatch.setattr(script.time, "sleep", lambda seconds: sleeps.append(seconds))

    assert script._inspect_manifest(f"{REGISTRY_BASE}:candidate") is None
    assert len(calls) == 1
    assert calls[0]["timeout"] == script._MANIFEST_INSPECT_TIMEOUT_SECONDS
    assert sleeps == []


def test_manifest_inspect_retries_a_timed_out_client(monkeypatch) -> None:
    script = _load_script()
    timeouts: list[int] = []
    sleeps: list[int] = []

    def run(cmd, **kwargs):
        timeouts.append(kwargs["timeout"])
        raise script.subprocess.TimeoutExpired(cmd, kwargs["timeout"])

    monkeypatch.setattr(script.subprocess, "run", run)
    monkeypatch.setattr(script.time, "sleep", lambda seconds: sleeps.append(seconds))

    assert (
        script._inspect_manifest(
            f"{REGISTRY_BASE}:candidate", attempts=2, initial_delay=1
        )
        is None
    )
    assert timeouts == [script._MANIFEST_INSPECT_TIMEOUT_SECONDS] * 2
    assert sleeps == [1]


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
