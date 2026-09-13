"""Which catalog hooks ship to which repo, and which of them can ever fire.

The applier writes this set into every consumer block and the coverage audit
checks against it. A second derivation of either fact makes the two disagree
without erroring, which is what let agentic-os#7628 stand. See
docs/pre-commit-hygiene.md.
"""
from __future__ import annotations

from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
HOOKS_FILE = REPO_ROOT / ".pre-commit-hooks.yaml"

# Org dirs of upstream checkouts nobody here owns, so the rollout writes
# nothing into them and the audit expects nothing of them.
VENDOR_ORGS = {"StrangeLoopGames"}

# Hand-editable. DEFAULT_REV tracks the newest tag on its own, so REMOVING an
# id here must land with the release that drops it.
DEFAULT_HOOK_IDS = [
    "catalog-trifecta",
    "documentation-placement",
    "documentation-size",
    "context-load-points",
    "code-comments",
    "actions-run-one-line",
    "source-doc-refs",
    "check-skills",
    "dead-cross-links",
    "repo-pointer-skills",
    "misplaced-skills",
    "agent-compose-size",
    "agent-compose-dedup",
    "trufflehog",
    "pr-guard",
    # Three that enforce rules the global AGENTS.md already binds everywhere,
    # wired here since #937 and rolled out only now. Consumers see new failures.
    "brand-case",
    "leak-guard",
    "unresolved-placeholder-guard",
    # Added last: aos wired this one locally and consumers never got it, so the
    # block drifted everywhere while the authoring repo stayed current (#937).
    "git-workflow",
]

# Per-repo hook opt-outs. eco-* repos vendor the Strange Loop Games Unity SDK,
# whose comments are not ours to lint. lore is a docs-only slice.
PER_REPO_HOOK_SKIPS: dict[str, set[str]] = {
    "lore": {
        "check-skills",
        "repo-pointer-skills",
        "misplaced-skills",
        "agent-compose-size",
        "agent-compose-dedup",
    },
}
# typos is absent by design: managed_block() emits it unconditionally, so an
# entry here never fires (#1155). Vendored trees go in the repo's _typos.toml.
ECO_HOOK_SKIPS = {"code-comments"}


def hook_ids_for(repo: str) -> list[str]:
    skips: set[str] = set(PER_REPO_HOOK_SKIPS.get(repo, set()))
    # The hyphen matters: a bare "eco" prefix also matches ecommerce-shaped
    # names that have nothing to do with the Eco game (agentic-os#7635).
    if repo.startswith("eco-"):
        skips |= ECO_HOOK_SKIPS
    return [h for h in DEFAULT_HOOK_IDS if h not in skips]


def ships_to(repo_dir: Path) -> bool:
    """Whether the rollout writes into this checkout at all.

    The audit asks the same question, so a vendor clone cannot be reported as
    missing hooks the applier refuses to send it (agentic-os#7635).
    """
    return repo_dir.parent.name not in VENDOR_ORGS


def hook_stages(hooks_file: Path | None = None) -> dict[str, list[str]]:
    """Declared stages per hook id, straight from the catalog definition."""
    import yaml

    path = hooks_file or HOOKS_FILE
    data = yaml.safe_load(path.read_text(encoding="utf-8")) or []
    return {h["id"]: list(h.get("stages") or []) for h in data if "id" in h}


def manual_only_ids(hooks_file: Path | None = None) -> set[str]:
    """Ids pre-commit runs only under `--hook-stage manual`."""
    return {
        hook_id
        for hook_id, stages in hook_stages(hooks_file).items()
        if stages and set(stages) <= {"manual"}
    }


def undeclared_shipped_ids(hooks_file: Path | None = None) -> list[str]:
    """Shipped ids the catalog does not define at all, so pre-commit errors."""
    declared = set(hook_stages(hooks_file))
    return [h for h in DEFAULT_HOOK_IDS if h not in declared]


def inert_shipped_ids(hooks_file: Path | None = None) -> list[str]:
    """Shipped ids that are manual-only, so every consumer carries a dead line.

    The rollout and the stage declaration have no shared check, so an id can sit
    in both and never run. docs/pre-commit-hygiene.md.
    """
    manual = manual_only_ids(hooks_file)
    return [h for h in DEFAULT_HOOK_IDS if h in manual]
