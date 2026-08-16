#!/usr/bin/env python3
"""Enforce the agent context-loading load-point framework, per-repo.

The three harnesses (Claude Code, Codex CLI, OpenCode) each load context from
the AGENTS.md / USER.md / CLAUDE.md tree. The framework says: one load point per
harness, share by symlink never fork, and let Claude bridge to AGENTS.md
through a one-line CLAUDE.md pointer. This hook enforces the
slice of that framework that is visible inside a single repository:

    1. Pure-pointer CLAUDE.md. A real (non-symlink) CLAUDE.md may contain only
       `@import` lines and blank lines - no forked doctrine. CLAUDE.md is the
       Claude->AGENTS bridge, not a place to restate operating context.

    2. No forked intermediate rungs. AGENTS.md and CLAUDE.md may live at the
       repo root only. A copy buried in a subdirectory is a forked rung that no
       harness loads cleanly. The exceptions are sharing and illustration: a
       symlink (the cross-harness sharing mechanism, e.g. an OpenCode workspace
       symlinking AGENTS.md to canonical) is fine, and load-point filenames that
       appear inside skill folders or examples/ trees are documentation, not
       loaded rungs.

    3. Require the canonical bridge. If the repo root has an AGENTS.md, it must
       also have a CLAUDE.md whose sole import is `@AGENTS.md`, so Claude Code
       always bridges from its load point into the shared doctrine.

Opt out per-repo via config: set `enabled = false` under the
[tool.agentic-os.context-load-points] section (e.g. a non-repository notes directory whose
CLAUDE.md is deliberately a memory file, not a pointer). See
docs/features-agents.md for the load-point overview.
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

from agentic_os.pre_commit.check_documentation_layout import (
    is_under_examples,
    is_under_skill_path,
    should_skip,
)
from agentic_os.config import is_enabled, is_excluded, load_excludes

REPO_ROOT = Path.cwd()
HOOK_ID = "context-load-points"

LOAD_POINT_NAMES = {"AGENTS.md", "CLAUDE.md"}
BRIDGE_TARGET = "AGENTS.md"

# A pointer line is a single Claude @-import: `@<path>` with no trailing prose.
_IMPORT_RE = re.compile(r"^@(\S+)$")


def imports_of(text: str) -> list[str]:
    """Return the @-import targets in a file, in order."""
    out: list[str] = []
    for line in text.splitlines():
        match = _IMPORT_RE.match(line.strip())
        if match:
            out.append(match.group(1))
    return out


def is_pure_pointer(text: str) -> bool:
    """True if every non-blank line is an @-import (no prose, headings, tables)."""
    for line in text.splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        if not _IMPORT_RE.match(stripped):
            return False
    return True


def load_point_files(root: Path, excludes: list[str]) -> list[Path]:
    out: list[Path] = []
    for name in sorted(LOAD_POINT_NAMES):
        for path in root.rglob(name):
            rel = path.relative_to(root)
            if should_skip(rel):
                continue
            if is_excluded(rel, excludes):
                continue
            out.append(rel)
    return sorted(out)


def check_pure_pointer(root: Path, rel: Path) -> list[str]:
    if rel.name != "CLAUDE.md":
        return []
    if is_under_skill_path(rel) or is_under_examples(rel):
        return []
    path = root / rel
    if path.is_symlink():
        return []
    text = path.read_text(encoding="utf-8", errors="replace")
    if is_pure_pointer(text):
        return []
    return [
        f"{rel.as_posix()}: CLAUDE.md must be a pure @-import pointer (only `@<file>` lines "
        f"and blank lines), not forked doctrine. Move operating context into "
        f"AGENTS.md and leave CLAUDE.md as `@AGENTS.md`."
    ]


def check_no_forked_rung(root: Path, rel: Path) -> list[str]:
    if len(rel.parts) == 1:
        return []
    if is_under_skill_path(rel) or is_under_examples(rel):
        return []
    if (root / rel).is_symlink():
        return []
    return [
        f"{rel.as_posix()}: {rel.name} may live only at the repo root. A real copy in a "
        f"subdirectory is a forked load-point rung. Symlink it to the canonical "
        f"file (sharing) or remove it."
    ]


def check_canonical_bridge(root: Path) -> list[str]:
    agents = root / "AGENTS.md"
    if not agents.exists():
        return []
    claude = root / "CLAUDE.md"
    if not claude.exists():
        return [
            "CLAUDE.md: repo root has AGENTS.md but no CLAUDE.md bridge. Add a "
            "CLAUDE.md containing exactly `@AGENTS.md` so Claude Code loads the "
            "shared doctrine."
        ]
    if claude.is_symlink():
        return []
    text = claude.read_text(encoding="utf-8", errors="replace")
    if BRIDGE_TARGET not in imports_of(text):
        return [
            "CLAUDE.md: root CLAUDE.md must bridge to AGENTS.md via `@AGENTS.md`. "
            "It does not import the sibling AGENTS.md."
        ]
    return []


def find_violations(root: Path) -> list[str]:
    excludes = load_excludes(HOOK_ID, repo_root=root)
    violations: list[str] = []
    for rel in load_point_files(root, excludes):
        violations += check_pure_pointer(root, rel)
        violations += check_no_forked_rung(root, rel)
    violations += check_canonical_bridge(root)
    return violations


def main() -> int:
    if not is_enabled(HOOK_ID):
        print(f"{HOOK_ID}: disabled by repo config")
        return 0
    violations = find_violations(REPO_ROOT)
    if not violations:
        print("context-load-points check: OK")
        return 0
    for violation in violations:
        sys.stderr.write(f"FAIL: {violation}\n")
    sys.stderr.write(f"\n{len(violations)} context load-point violation(s).\n")
    return 1


if __name__ == "__main__":
    sys.exit(main())
