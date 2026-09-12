#!/usr/bin/env python3
"""Inject or refresh the managed git-workflow block in each repo's AGENTS.md.

The authored applier; the renderer and drift check live in
agentic_os.generators.generate_git_workflow. Idempotent, and each block renders
from the lane that repo declares as `ward.workflow`, so there is no per-repo
argument and no remote lookup. The fleet rollout belongs in infrastructure per
the authoring-versus-rollout split. See docs/features-agents.md.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from agentic_os import config as cfg  # noqa: E402
from agentic_os.generators.generate_git_workflow import (  # noqa: E402
    BODY_FULL,
    apply_to_text,
    detect_lane,
    resolve_body,
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
    # Read the mode from the repo being written, never from the applier's cwd.
    body = resolve_body(repo_dir)
    detail = lane if body == BODY_FULL else f"{lane}, {body} body"
    after = apply_to_text(before, body)
    if after == before:
        return "ok", f"already current ({detail})"
    if not dry_run:
        agents.write_text(after, encoding="utf-8", newline="\n")
    return ("would-write" if dry_run else "wrote"), detail


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
