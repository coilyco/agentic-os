#!/usr/bin/env python3
"""Validate role-composed skill sources before agent-compose can promote them."""

from __future__ import annotations

import sys
from pathlib import Path

from agentic_os.config import is_enabled
from agentic_os.pre_commit import check_skill

HOOK_ID = "check-composed-skills"


def layout_problems(repo_root: Path) -> list[str]:
    composed = repo_root / ".agents" / "composed"
    if not composed.is_dir():
        return []
    ordinary = repo_root / ".agents" / "skills"
    if not (ordinary / "categories.yaml").is_file():
        return [
            ".agents/composed: role-composed sources require "
            ".agents/skills/categories.yaml"
        ]
    ordinary_names = {
        entry.name
        for entry in ordinary.iterdir()
        if entry.is_dir() and not entry.name.startswith(".")
    }
    problems: list[str] = []
    for linked in sorted(path for path in composed.rglob("*") if path.is_symlink()):
        problems.append(
            f"{linked.relative_to(repo_root)}: symlinks are invalid inside "
            "role-composed sources"
        )
    for leaked in sorted(composed.rglob("SKILL.md")):
        problems.append(
            f"{leaked.relative_to(repo_root)}: composed sources must use COMPOSED.md"
        )
    for entry in sorted(composed.iterdir()):
        if entry.name.startswith("."):
            continue
        rel = entry.relative_to(repo_root)
        if entry.is_symlink() or not entry.is_dir():
            problems.append(f"{rel}: composed entries must be real directories")
            continue
        if entry.name in ordinary_names:
            problems.append(
                f"{rel}: name collides with .agents/skills/{entry.name}"
            )
        if not (entry / "COMPOSED.md").is_file():
            problems.append(f"{rel}: missing COMPOSED.md")
    return problems


def main(argv: list[str] | None = None) -> int:
    if not is_enabled(HOOK_ID):
        print(f"{HOOK_ID}: disabled by repo config")
        return 0
    if argv is None:
        argv = sys.argv
    repo_root = Path.cwd()
    composed = repo_root / ".agents" / "composed"
    if not composed.is_dir():
        return 0
    problems = layout_problems(repo_root)
    if problems:
        for problem in problems:
            sys.stderr.write(f"FAIL: {problem}\n")
        sys.stderr.write(f"\n{len(problems)} composed skill violation(s).\n")
        return 1
    return check_skill.main(
        [
            argv[0],
            "--skills-dir",
            ".agents/composed",
            "--entrypoint",
            "COMPOSED.md",
            "--spec-path",
            ".agents/skills/categories.yaml",
            "--reference-skills-dir",
            ".agents/skills",
            *argv[1:],
        ],
        hook_id=HOOK_ID,
    )


if __name__ == "__main__":
    sys.exit(main())
