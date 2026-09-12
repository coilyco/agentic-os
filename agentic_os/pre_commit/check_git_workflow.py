#!/usr/bin/env python3
"""Assert AGENTS.md carries the managed git-workflow block, never hand-edited.

Regenerates it from the lane the same file declares as `ward.workflow` and fails
on drift, a missing block, or one that no longer matches the lane. Unlike the
workspace-pointer hook it exempts no base repo: a landing lane binds an agent in
the canonical base exactly as it binds one in a consumer.
"""

from __future__ import annotations

import sys
from pathlib import Path
from typing import NoReturn

from agentic_os.config import is_enabled
from agentic_os.generators.generate_git_workflow import check_drift, resolve_body

HOOK_ID = "git-workflow"
TRACKER = "docs/features-agents.md"

REGEN_HINT = "  regenerate: python3 scripts/apply-git-workflow.py --repo <name>"


def fail(msgs: list[str]) -> NoReturn:
    for m in msgs:
        print(f"check-{HOOK_ID}: {m}", file=sys.stderr)
    print(REGEN_HINT, file=sys.stderr)
    print(f"  see {TRACKER} for schema", file=sys.stderr)
    sys.exit(1)


def main() -> int:
    if not is_enabled(HOOK_ID):
        print(f"{HOOK_ID}: disabled by repo config")
        return 0

    agents = Path.cwd() / "AGENTS.md"
    if not agents.exists() or agents.is_symlink():
        return 0

    problems = check_drift(
        agents.read_text(encoding="utf-8", errors="replace"), resolve_body()
    )
    if problems:
        fail(problems)
    print(f"{HOOK_ID} check: OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
