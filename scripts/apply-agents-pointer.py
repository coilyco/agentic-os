#!/usr/bin/env python3
"""Inject or refresh the managed workspace-pointer block in each repo's AGENTS.md.

Authored side of the AGENTS.md pointer convention. The pure renderer and the
drift check live in `agentic_os.generators.generate_agents_pointer`; this script is the
applier that writes the block into a repo's `AGENTS.md` in place. Idempotent:
re-runs replace the prior managed block and strip known legacy intro lines, so
running it twice is a no-op.

Org-aware (each org points at its own base, on the host fitting its trust tier):
  coilyco-flight-deck/* -> public base on GitHub.
  coilyco-bridge/*      -> public base on GitHub + private agentic-os-kai overlay
                           on Forgejo, layered.
Unmanaged orgs (coilysiren/*, non-coilyco remotes) and the canonical base repos
themselves (agentic-os, agentic-os-kai) are skipped.

This is the AUTHORED tool. The fleet rollout that lands the block on each
managed repo's canonical `main` is `scripts/agents-pointer-migrate.py` in
infrastructure (`ward exec agents-pointer-migrate`), per the authoring-vs-rollout
split - not this script run by hand across the fleet. (The earlier report-only
ansible `agents-pointer` role was retired in infrastructure#362.)

Usage:
    python3 scripts/apply-agents-pointer.py --dry-run   # show the plan
    python3 scripts/apply-agents-pointer.py             # apply to all managed repos
    python3 scripts/apply-agents-pointer.py --repo luca # one repo by name

See docs/features-agents.md.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from agentic_os import config as cfg  # noqa: E402
from agentic_os.generators.generate_agents_pointer import (  # noqa: E402
    EXEMPT,
    apply_to_text,
    detect_org_repo,
    is_managed,
)


def apply_to_repo(repo_dir: Path, dry_run: bool) -> tuple[str, str]:
    """Apply the block to one repo. Returns (action, detail) for reporting."""
    org, repo = detect_org_repo(repo_dir)
    if (org, repo) in EXEMPT:
        return "exempt", f"{org}/{repo} is a canonical base"
    if not is_managed(org, repo):
        return "skip", f"unmanaged org {org!r}"
    agents = repo_dir / "AGENTS.md"
    if not agents.exists():
        return "skip", "no AGENTS.md"
    if agents.is_symlink():
        return "skip", "symlinked AGENTS.md"

    assert org is not None
    before = agents.read_text()
    after = apply_to_text(before, org)
    if after is None:
        return "skip", "unmanaged"
    if after == before:
        return "ok", "already current"
    if not dry_run:
        agents.write_text(after)
    return ("would-write" if dry_run else "wrote"), org


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

    print(f"Applying AGENTS.md pointer block to {len(repos)} repo(s)")
    if args.dry_run:
        print("(dry run)")
    print()

    counts: dict[str, int] = {}
    for repo_dir in repos:
        action, detail = apply_to_repo(repo_dir, args.dry_run)
        counts[action] = counts.get(action, 0) + 1
        if action not in ("skip", "exempt"):
            print(f"  {repo_dir.name:24} {action:12} {detail}")

    print()
    print("Summary:", ", ".join(f"{k}={v}" for k, v in sorted(counts.items())))
    return 0


if __name__ == "__main__":
    sys.exit(main())
