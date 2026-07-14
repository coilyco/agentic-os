"""Shared dev-base tier metadata and release/build planning helpers."""
from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent
DEV_BASE_ROOT = REPO_ROOT / "docker" / "dev-base"
REGISTRY_BASE = "forgejo.coilysiren.me/coilyco-flight-deck/agentic-os"
LOCAL_REGISTRY_BASE = "agentic-os"


@dataclass(frozen=True)
class TierSpec:
    tier: str
    stage: str
    dockerfile: Path
    base_tier: str | None
    # Sibling tiers grafted in via COPY --from of their self-contained install
    # dirs - a composed tier's extra toolchains (docs/dev-base-image-tiering.md).
    graft_tiers: tuple[str, ...] = ()
    published: bool = True


# TIER_SPECS stays in topological order: every base_tier and graft_tier names
# an earlier entry, so a sequential local build always has its inputs.
TIER_SPECS: tuple[TierSpec, ...] = (
    TierSpec(
        tier="core",
        stage="dev-base-core",
        dockerfile=DEV_BASE_ROOT / "core" / "Dockerfile",
        base_tier=None,
    ),
    TierSpec(
        tier="lang-node",
        stage="dev-base-lang-node",
        dockerfile=DEV_BASE_ROOT / "lang-node" / "Dockerfile",
        base_tier="core",
    ),
    TierSpec(
        tier="lang-go",
        stage="dev-base-lang-go",
        dockerfile=DEV_BASE_ROOT / "lang-go" / "Dockerfile",
        base_tier="core",
    ),
    TierSpec(
        tier="lang-dotnet",
        stage="dev-base-lang-dotnet",
        dockerfile=DEV_BASE_ROOT / "lang-dotnet" / "Dockerfile",
        base_tier="core",
    ),
    TierSpec(
        tier="ops",
        stage="dev-base-ops",
        dockerfile=DEV_BASE_ROOT / "ops" / "Dockerfile",
        base_tier="core",
    ),
    TierSpec(
        tier="agent",
        stage="dev-base-agent",
        dockerfile=DEV_BASE_ROOT / "agent" / "Dockerfile",
        base_tier="ops",
        graft_tiers=("lang-node",),
    ),
    TierSpec(
        tier="full",
        stage="dev-base-full",
        dockerfile=DEV_BASE_ROOT / "full" / "Dockerfile",
        base_tier="agent",
        graft_tiers=("lang-go", "lang-dotnet"),
    ),
)

TIER_BY_NAME = {tier.tier: tier for tier in TIER_SPECS}
PUBLISHED_TIER_NAMES = tuple(tier.tier for tier in TIER_SPECS if tier.published)


def tier_tag(tier: str, tag: str) -> str:
    # One package, variants as tags: full is the plain default tag, every
    # other tier rides a "<tier>-" prefix on the same agentic-os name.
    if tier == "full":
        return tag
    return f"{tier}-{tag}"


def image_ref(registry_base: str, tier: str, tag: str) -> str:
    return f"{registry_base}:{tier_tag(tier, tag)}"


def legacy_full_image_ref(registry_base: str, tag: str) -> str:
    return f"{registry_base}-full:{tag}"


def tier_dockerfile(tier: str) -> Path:
    return TIER_BY_NAME[tier].dockerfile


def graft_build_arg(tier: str) -> str:
    return f"{tier.replace('-', '_').upper()}_IMAGE"


def publish_plan(
    registry_base: str, tag: str, alias: str | None = None
) -> list[dict[str, str | bool | dict[str, str]]]:
    plan: list[dict[str, str | bool | dict[str, str]]] = []
    images: dict[str, str] = {}
    for spec in TIER_SPECS:
        ref = image_ref(registry_base, spec.tier, tag)
        base_ref = "ubuntu:24.04" if spec.base_tier is None else images[spec.base_tier]
        entry: dict[str, str | bool | dict[str, str]] = {
            "tier": spec.tier,
            "stage": spec.stage,
            "dockerfile": spec.dockerfile.relative_to(REPO_ROOT).as_posix(),
            "image": ref,
            "cache_image": image_ref(registry_base, spec.tier, "buildcache"),
            "base_image": base_ref,
            "graft_images": {
                graft_build_arg(graft): images[graft] for graft in spec.graft_tiers
            },
            "published": spec.published,
        }
        if alias:
            entry["alias_image"] = image_ref(registry_base, spec.tier, alias)
        if spec.tier == "full":
            entry["legacy_alias_image"] = legacy_full_image_ref(registry_base, "latest")
        plan.append(entry)
        images[spec.tier] = ref
    return plan
