"""Tests for the cached dev-base language-payload contract."""

from __future__ import annotations

import re
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent
DEV_BASE_ROOT = REPO_ROOT / "docker" / "dev-base"
DOCKERFILE = DEV_BASE_ROOT / "Dockerfile"
FULL_DOCKERFILE = DEV_BASE_ROOT / "full" / "Dockerfile"
INSTALL_COMMON = DEV_BASE_ROOT / "install-common.sh"
VERIFY_COMMON = DEV_BASE_ROOT / "verify-common.sh"


def _language_stages(text: str) -> tuple[str, ...]:
    return tuple(
        re.findall(r"^FROM ubuntu:\S+ AS (dev-base-lang-\S+)$", text, re.MULTILINE)
    )


def _stage_text(text: str, stage: str) -> str:
    match = re.search(rf"^FROM ubuntu:\S+ AS {re.escape(stage)}$", text, re.MULTILINE)
    assert match is not None
    remainder = text[match.start() :]
    next_stage = remainder.find("\nFROM ", 1)
    return remainder if next_stage < 0 else remainder[:next_stage]


def test_language_targets_are_direct_independent_ubuntu_descendants() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8")
    stages = _language_stages(text)

    assert stages
    for stage_name in stages:
        stage = _stage_text(text, stage_name)
        assert "ARG BASE_IMAGE" not in stage
        assert "FROM ${BASE_IMAGE}" not in stage


def test_language_targets_are_payload_only_and_share_architecture_mapping() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8")
    stage_count = len(_language_stages(text))

    assert stage_count == 5
    assert text.count("source=write-arch-env.sh,target=/tmp/write-arch-env.sh") == stage_count
    for forbidden in (
        "install-common.sh",
        "verify-common.sh",
        "repo-lists",
        "ward-shell-entrypoint.sh",
        "AGENT_COMPOSE_VERSION",
        "WARD_VERSION",
        "SPECGEN_VERSION",
        "AOS_VERSION",
    ):
        assert forbidden not in text


def test_full_image_owns_the_common_and_internal_tool_surface() -> None:
    text = FULL_DOCKERFILE.read_text(encoding="utf-8")

    assert text.count("source=install-common.sh,target=/tmp/install-common.sh") == 1
    assert text.count("source=verify-common.sh,target=/tmp/verify-common.sh") == 1
    assert "source=/opt/agentic-os/substrate-repos.txt" in text
    assert '["/opt/agentic-os/ward-shell-entrypoint.sh"]' in text
    assert "COPY --from=dev-base-tool-builder /opt/agentic-os/aosguard-skill" in text
    assert "COPY --from=dev-base-tool-builder /opt/agentic-os/agentic_os" in text
    for name in ("AGENT_COMPOSE_VERSION", "WARD_VERSION", "SPECGEN_VERSION", "AOS_VERSION"):
        assert f"ARG {name}=" in text


def test_aos_is_installed_from_a_versioned_release() -> None:
    text = FULL_DOCKERFILE.read_text(encoding="utf-8")

    assert re.search(r"^ARG AOS_VERSION=\d+\.\d+\.\d+$", text, re.MULTILINE)
    assert "releases/download/aos-v${AOS_VERSION}" in text
    assert 'aos_asset="aos-bundle-linux-${TARGETARCH}.tar.gz"' in text
    assert "go build -trimpath -ldflags \"-s -w -X main.version=dev-base\"" not in text
    assert "COPY --from=aos-cli" not in text


def test_aosguard_is_pinned_rather_than_compiled_from_the_tree() -> None:
    """The image builds nothing from this repo, so `docker/**` is an honest filter.

    aosguard used to be compiled in-image from `.specgen`, which made three
    build contexts image inputs that the path filter never watched. See
    docs/dev-base-image.md.
    """
    text = FULL_DOCKERFILE.read_text(encoding="utf-8")
    bake = (DEV_BASE_ROOT / "docker-bake.hcl").read_text(encoding="utf-8")

    for gone in ("aosguard-spec", "aosguard-python", "repo-lists"):
        assert gone not in text, f"{gone} is still a Dockerfile build context"
        assert gone not in bake, f"{gone} is still a bake build context"

    assert "specgen --project-root" not in text, "aosguard is compiled in-image again"
    assert '--set-version "dev-base"' not in text
    # Everything the compile used to produce now arrives in one verified bundle.
    assert "/tmp/aos-bundle/bin/aosguard" in text
    assert "/tmp/aos-bundle/share/aos/aosguard-skill" in text
    assert "/tmp/aos-bundle/share/aos/python/agentic_os" in text
    assert "/tmp/aos-bundle/share/aos/repositories/substrate-repos.txt" in text


def test_ward_is_installed_from_a_checksummed_release_not_built_from_source() -> None:
    text = FULL_DOCKERFILE.read_text(encoding="utf-8")

    assert 'ward_asset="ward-linux-${TARGETARCH}"' in text
    assert '"${ward_base}/SHA256SUMS"' in text
    assert "sha256sum -c -" in text
    assert "git clone" not in text
    assert "go build" not in text


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
    # aos#771: the isolated-import proof is the only thing standing between a
    # pinned hook rev and the image copy, so it may not quietly disappear.
    assert 'printf \'SENTINEL = "isolated"\\n\'' in text
    assert "import agentic_os; print(agentic_os.SENTINEL)" in text
    assert "test -s /opt/agentic-os/aosguard-skill/aosguard/SKILL.md" in text
    assert "references/commands.yaml" in text


def test_substrate_seed_parser_accepts_windows_line_endings() -> None:
    text = INSTALL_COMMON.read_text(encoding="utf-8")

    assert "ref=${ref%$'\\r'}" in text


def test_version_defaults_have_one_owning_source() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8") + FULL_DOCKERFILE.read_text(
        encoding="utf-8"
    )
    names = re.findall(r"^ARG ([A-Z0-9_]+)=", text, re.MULTILINE)

    assert len(names) == len(set(names))
    assert "AOS_VERSION" in names


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
    text = FULL_DOCKERFILE.read_text(encoding="utf-8")

    assert "FROM ${BASE_IMAGE} AS dev-base-full" in text
    assert "DOTNET_ROOT=/usr/local/dotnet" in text
    assert "COPY --from=dev-base-lang-node-graft /usr/local/node /usr/local/node" in text
    assert "COPY --from=dev-base-lang-go-graft /usr/local/go /usr/local/go" in text
    assert "COPY --from=dev-base-lang-dotnet-graft /usr/local/dotnet /usr/local/dotnet" in text
    assert "COPY --from=dev-base-lang-python-graft /opt/uv /opt/uv" in text
