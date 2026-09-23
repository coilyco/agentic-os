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

# A repo carrying this file has opted out, fail-closed.
IGNORE_MARKER = ".agentic-os-ignore"

# The authoring repo wires every validator as `repo: local`, so the rollout
# writes it no block and it still owes coverage. docs/pre-commit-hygiene.md.
SOURCE_REPO = "agentic-os"

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
    # Sibling of check-skills over .agents/composed, authored #1073 and never
    # shipped, so aosk grew six unselected sources green. Inert without one.
    "check-composed-skills",
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
    # housecast grades and composes nothing, so it carries no AGENTS.COMPOSE.md for
    # these to check, and their ids named a consumer it no longer knows about.
    "housecast": {"agent-compose-size", "agent-compose-dedup"},
    # check-skills left this set once lore committed a categories.yaml carrying
    # its declared 4000-char cap (teable:coilyco-bridge/lore#7753).
    "lore": {
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


def opted_out(repo_dir: Path) -> tuple[bool, str]:
    """Whether this checkout is outside the suite entirely, and why.

    Vendor clones and marker files owe no coverage, so the audit reports them
    as exempt rather than as gaps (agentic-os#7628, #7635, #7638).

    This is the one gate for every fleet-wide mutator in this repo, and any new
    one calls it before it reads or writes. scripts/apply-git-workflow.py did
    not, and wrote a managed block into a repo carrying the marker
    (agentic-os#6894), which is the failure a shared helper exists to prevent.
    """
    if repo_dir.parent.name in VENDOR_ORGS:
        return True, f"vendor org ({repo_dir.parent.name})"
    if (repo_dir / IGNORE_MARKER).exists():
        return True, f"opted out ({IGNORE_MARKER})"
    return False, ""


def ships_to(repo_dir: Path) -> bool:
    """Whether the rollout renders the managed block into this checkout.

    Narrower than owing coverage: the source repo gets no block and is still
    audited, because a hand-maintained config drifts too (agentic-os#7634).
    """
    return not opted_out(repo_dir)[0] and repo_dir.name != SOURCE_REPO


# A hook that reads a spec file checks nothing without it and pre-commit still
# renders it Passed. Roots mirror check_skill.detect_skills_dir, longest first.
SPEC_REQUIRED: dict[str, tuple[tuple[str, ...], str, str]] = {
    "check-skills": (
        (".agents/skills", ".claude/skills", "skills"),
        "categories.yaml",
        "SKILL.md",
    ),
}


def unarmed_spec_hooks(repo_dir: Path, referenced: set[str]) -> list[str]:
    """Ids this repo runs that have no spec to read, so they evaluate nothing.

    Distinct from a missing hook, which is visibly absent, and from a
    manual-only one, which never runs. This one runs, passes, and checks
    nothing, so only a filesystem look tells it apart from real coverage.
    """
    from agentic_os.generators.generate_repo_pointer_skill import SKILL_PREFIX

    out: list[str] = []
    for hook_id, (roots, spec_name, entrypoint) in sorted(SPEC_REQUIRED.items()):
        if hook_id not in referenced:
            continue
        for root in roots:
            base = repo_dir / root
            if not base.is_dir():
                continue
            owned = [p for p in base.glob(f"*/{entrypoint}")]
            # repo-pointer-skills owns a generated pointer, so a tree of
            # nothing else has no check going unperformed. docs/FEATURES.md.
            hand_written = [
                p for p in owned if not p.parent.name.startswith(SKILL_PREFIX)
            ]
            if hand_written and not (base / spec_name).is_file():
                out.append(f"{hook_id}: no {root}/{spec_name}")
            break
    return out


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
