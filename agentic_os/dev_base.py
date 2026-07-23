"""Shared dev-base tier metadata and release/build planning helpers."""
from __future__ import annotations

from collections.abc import Iterable
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
    shared_context: bool = False
    published: bool = True


# TIER_SPECS stays in topological order: every base_tier and graft_tier names
# an earlier entry, so a sequential local build always has its inputs.
TIER_SPECS: tuple[TierSpec, ...] = (
    TierSpec(
        tier="core",
        stage="dev-base-core",
        dockerfile=DEV_BASE_ROOT / "core" / "Dockerfile",
        base_tier=None,
        shared_context=True,
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
        tier="lang-rust",
        stage="dev-base-lang-rust",
        dockerfile=DEV_BASE_ROOT / "lang-rust" / "Dockerfile",
        base_tier="core",
    ),
    TierSpec(
        tier="lang-python",
        stage="dev-base-lang-python",
        dockerfile=DEV_BASE_ROOT / "lang-python" / "Dockerfile",
        base_tier="core",
    ),
    TierSpec(
        tier="full",
        stage="dev-base-full",
        dockerfile=DEV_BASE_ROOT / "full" / "Dockerfile",
        # full inherits the complete native Rust surface. The remaining
        # language toolchains are self-contained prefix grafts.
        base_tier="lang-rust",
        graft_tiers=(
            "lang-go",
            "lang-dotnet",
            "lang-python",
        ),
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


def tier_dockerfile(tier: str) -> Path:
    return TIER_BY_NAME[tier].dockerfile


def graft_build_arg(tier: str) -> str:
    return f"{tier.replace('-', '_').upper()}_IMAGE"


def normalize_aliases(aliases: str | Iterable[str] | None = None) -> tuple[str, ...]:
    if aliases is None:
        return ()

    if isinstance(aliases, str):
        raw_aliases = (aliases,)
    else:
        raw_aliases = tuple(aliases)

    normalized: list[str] = []
    seen: set[str] = set()
    for alias in raw_aliases:
        clean_alias = str(alias).strip()
        if not clean_alias or clean_alias in seen:
            continue
        normalized.append(clean_alias)
        seen.add(clean_alias)
    return tuple(normalized)


def publish_plan(
    registry_base: str, tag: str, aliases: str | Iterable[str] | None = None
) -> list[dict[str, object]]:
    plan: list[dict[str, object]] = []
    alias_tags = normalize_aliases(aliases)
    images: dict[str, str] = {}
    for spec in TIER_SPECS:
        ref = image_ref(registry_base, spec.tier, tag)
        base_ref = "ubuntu:24.04" if spec.base_tier is None else images[spec.base_tier]
        entry: dict[str, object] = {
            "tier": spec.tier,
            "stage": spec.stage,
            "dockerfile": spec.dockerfile.relative_to(REPO_ROOT).as_posix(),
            # Most tiers build from their own folder. Core owns the shared
            # agent assets, so its build uses docker/dev-base root.
            "context_dir": (
                spec.dockerfile.parent.parent
                if spec.shared_context
                else spec.dockerfile.parent
            ).relative_to(REPO_ROOT).as_posix(),
            "image": ref,
            "cache_image": image_ref(registry_base, spec.tier, "buildcache"),
            "base_image": base_ref,
            "graft_images": {
                graft_build_arg(graft): images[graft] for graft in spec.graft_tiers
            },
            "published": spec.published,
        }
        if alias_tags:
            alias_images = [image_ref(registry_base, spec.tier, alias) for alias in alias_tags]
            entry["alias_images"] = alias_images
            entry["alias_image"] = alias_images[0]
        plan.append(entry)
        images[spec.tier] = ref
    return plan
