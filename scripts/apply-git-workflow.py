#!/usr/bin/env python3
"""Inject or refresh the managed git-workflow block in each repo's AGENTS.md.

Authored side of the AGENTS.md git-workflow convention. The pure renderer and
the drift check live in `agentic_os.generators.generate_git_workflow`; this
script is the applier that writes the block into a repo's `AGENTS.md` in place.
Idempotent: re-runs replace the prior managed block and strip the legacy
one-line `**Git workflow** -` stamp, so running it twice is a no-op.

Lane-aware and org-agnostic. Each repo's block is rendered from the lane that
repo declares as `ward.workflow` in its own AGENTS.md frontmatter, so the
applier needs no per-repo argument and no remote lookup. A repo that declares
no lane gets the undeclared variant, which holds the agent to the
branch-and-pull-request shape rather than guessing at a direct push to `main`.

This is the AUTHORED tool. The fleet rollout that lands the block on each repo's
canonical `main` belongs in infrastructure alongside the pointer migration, per
the authoring-vs-rollout split, not this script run by hand across the fleet.

Usage:
    python3 scripts/apply-git-workflow.py --dry-run   # show the plan
    python3 scripts/apply-git-workflow.py             # apply to every repo
    python3 scripts/apply-git-workflow.py --repo ward # one repo by name

See docs/features-agents.md.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from agentic_os import config as cfg  # noqa: E402
from agentic_os.generators.generate_git_workflow import (  # noqa: E402
    apply_to_text,
    detect_lane,
)


def apply_to_repo(repo_dir: Path, dry_run: bool) -> tuple[str, str]:
    """Apply the block to one repo. Returns (action, detail) for reporting."""
    agents = repo_dir / "AGENTS.md"
    if not agents.exists():
        return "skip", "no AGENTS.md"
    if agents.is_symlink():
        return "skip", "symlinked AGENTS.md"

    before = agents.read_text(encoding="utf-8", errors="replace")
    lane = detect_lane(before) or "undeclared"
    after = apply_to_text(before)
    if after == before:
        return "ok", f"already current ({lane})"
    if not dry_run:
        agents.write_text(after, encoding="utf-8", newline="\n")
    return ("would-write" if dry_run else "wrote"), lane


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--repo", help="apply to a single repo by name")
    ap.add_argument("--skip", nargs="*", default=[])
    args = ap.parse_args(argv)

    all_dirs = cfg.iter_workspace_repos()
    if args.repo:
        repos = [d for d in all_dirs if d.name == args.repo]
        if not repos:
            print(f"No checked-out repo named {args.repo!r} under {cfg.projects_root()}")
            return 1
    else:
        skip = set(args.skip)
        repos = [d for d in all_dirs if d.name not in skip]

    print(f"Applying AGENTS.md git-workflow block to {len(repos)} repo(s)")
    if args.dry_run:
        print("(dry run)")
    print()

    counts: dict[str, int] = {}
    for repo_dir in repos:
        action, detail = apply_to_repo(repo_dir, args.dry_run)
        counts[action] = counts.get(action, 0) + 1
        if action != "skip":
            print(f"  {repo_dir.name:24} {action:12} {detail}")

    print()
    print("Summary:", ", ".join(f"{k}={v}" for k, v in sorted(counts.items())))
    return 0


if __name__ == "__main__":
    sys.exit(main())
