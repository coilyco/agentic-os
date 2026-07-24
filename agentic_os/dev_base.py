"""Shared metadata and release planning for the full dev-base image."""

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
    """Compatibility-shaped metadata for the sole published image."""

    tier: str
    stage: str
    dockerfile: Path


TIER_SPECS: tuple[TierSpec, ...] = (
    TierSpec(
        tier="full",
        stage="dev-base-full",
        dockerfile=DEV_BASE_ROOT / "Dockerfile",
    ),
)
TIER_BY_NAME = {spec.tier: spec for spec in TIER_SPECS}
PUBLISHED_TIER_NAMES = ("full",)


def tier_tag(tier: str, tag: str) -> str:
    if tier != "full":
        raise ValueError(f"unsupported dev-base image: {tier}")
    return tag


def image_ref(registry_base: str, tier: str, tag: str) -> str:
    return f"{registry_base}:{tier_tag(tier, tag)}"


def tier_dockerfile(tier: str) -> Path:
    return TIER_BY_NAME[tier].dockerfile


def normalize_aliases(aliases: str | Iterable[str] | None = None) -> tuple[str, ...]:
    if aliases is None:
        return ()

    raw_aliases = (aliases,) if isinstance(aliases, str) else tuple(aliases)
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
    spec = TIER_SPECS[0]
    ref = image_ref(registry_base, spec.tier, tag)
    entry: dict[str, object] = {
        "tier": spec.tier,
        "stage": spec.stage,
        "dockerfile": spec.dockerfile.relative_to(REPO_ROOT).as_posix(),
        "context_dir": DEV_BASE_ROOT.relative_to(REPO_ROOT).as_posix(),
        "image": ref,
        "cache_image": image_ref(registry_base, spec.tier, "buildcache"),
    }
    alias_tags = normalize_aliases(aliases)
    if alias_tags:
        alias_images = [
            image_ref(registry_base, spec.tier, alias) for alias in alias_tags
        ]
        entry["alias_images"] = alias_images
        entry["alias_image"] = alias_images[0]
    return [entry]
