#!/usr/bin/env python3
"""Assert every `repo-<name>` pointer skill is auto-generated, never hand-edited.

Regenerates each one offline from its own frontmatter and fails on drift in the
body or frontmatter, and on a description that skipped the cleaning step.
Prefix-driven and categories.yaml-independent, so it fires in every consumer repo
and no-ops where there is no skills surface. See docs/features-agents.md.
"""

from __future__ import annotations

import sys
from pathlib import Path
from typing import NoReturn

from agentic_os.config import get_str_option, is_enabled
from agentic_os.generators.generate_repo_pointer_skill import (
    DEFAULT_ORG,
    SKILL_PREFIX,
    check_drift,
)
from agentic_os.pre_commit.tree import carries_content

HOOK_ID = "repo-pointer-skills"
TRACKER = "docs/features-agents.md"

SKILLS_DIR_CANDIDATES = (".agents/skills", ".claude/skills", "skills")

REGEN_HINT = (
    "  regenerate: aosguard ops forgejo repo get <owner> <name> "
    "| python -m agentic_os.generators.generate_repo_pointer_skill <name> --from-json - --repo-root <repo>\n"
    "  on GitHub: gh repo view <owner>/<name> --json description,repositoryTopics "
    "| the same generator"
)


def fail(msgs: list[str]) -> NoReturn:
    for m in msgs:
        print(f"check-{HOOK_ID}: {m}", file=sys.stderr)
    print(REGEN_HINT, file=sys.stderr)
    print(f"  see {TRACKER} for schema", file=sys.stderr)
    sys.exit(1)


def find_skills_dir(root: Path) -> Path | None:
    for candidate in SKILLS_DIR_CANDIDATES:
        d = root / candidate
        if d.is_dir():
            return d
    return None


def main() -> int:
    if not is_enabled(HOOK_ID):
        print(f"{HOOK_ID}: disabled by repo config")
        return 0

    root = Path.cwd()
    skills_dir = find_skills_dir(root)
    if skills_dir is None:
        return 0

    org = get_str_option(HOOK_ID, "org", DEFAULT_ORG)
    failures: list[str] = []
    for d in sorted(skills_dir.glob(f"{SKILL_PREFIX}*")):
        if not d.is_dir() or d.is_symlink():
            continue
        if not carries_content(d.relative_to(root), root):
            continue
        skill_md = d / "SKILL.md"
        if not skill_md.is_file():
            failures.append(f"{d.name} has no SKILL.md")
            continue
        failures.extend(check_drift(d.name, skill_md.read_text(), org))

    if failures:
        fail(failures)
    return 0


if __name__ == "__main__":
    sys.exit(main())
