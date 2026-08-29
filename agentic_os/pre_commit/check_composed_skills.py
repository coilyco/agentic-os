#!/usr/bin/env python3
"""Validate role-composed skill sources before agent-compose can promote them.

Also pairs the catalogue with `.agents/roles.kdl` both ways: every role selects
at least one source, and every source is selected by at least one role. Neither
direction had a gate, so a role could go empty and a source could go unclaimed
with every hook still passing. Deliberate exceptions live in
`[tool.agentic-os.check-composed-skills] unselected`. See agentic-os#1073.
"""

from __future__ import annotations

import sys
from pathlib import Path

import fnmatch
import re

from agentic_os.config import is_enabled, load_str_list
from agentic_os.pre_commit import check_skill
from agentic_os.pre_commit.tree import carries_content

HOOK_ID = "check-composed-skills"
ROLE_GRAPH = Path(".agents") / "roles.kdl"
ROLE_OPEN_RE = re.compile(r'^role\s+"?([\w-]+)"?\s*\{')
SELECTOR_RE = re.compile(r'^composed-skill\s+"?([^"\s]+)"?\s*$')


def role_selectors(repo_root: Path) -> dict[str, list[str]] | None:
    """Each role's composed-skill selectors, or None when there is no graph."""
    path = repo_root / ROLE_GRAPH
    if not path.is_file():
        return None
    roles: dict[str, list[str]] = {}
    current: str | None = None
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if opened := ROLE_OPEN_RE.match(stripped):
            current = opened.group(1)
            roles.setdefault(current, [])
            continue
        if stripped == "}":
            current = None
            continue
        if current and (selector := SELECTOR_RE.match(stripped)):
            roles[current].append(selector.group(1))
    return roles


def catalogue_problems(repo_root: Path, composed: Path) -> list[str]:
    """Assert the role graph and the composed catalogue agree, both ways.

    A role with no selector composes nothing and every other hook still passes:
    the Executive Strategist shipped empty for eleven days that way. A source no
    role selects is dead weight nobody notices. See agentic-os#1073.
    """
    roles = role_selectors(repo_root)
    if roles is None:
        return []
    problems = [
        f"{ROLE_GRAPH.as_posix()}: role {name} has no composed-skill selector, "
        f"so it composes nothing"
        for name, selectors in sorted(roles.items())
        if not selectors
    ]
    every = [selector for selectors in roles.values() for selector in selectors]
    allowed = load_str_list(HOOK_ID, "unselected", repo_root)
    used: set[str] = set()
    for entry in sorted(composed.iterdir()):
        if not entry.is_dir() or entry.name.startswith("."):
            continue
        if not carries_content(entry.relative_to(repo_root), repo_root):
            continue
        if any(fnmatch.fnmatchcase(entry.name, selector) for selector in every):
            continue
        covering = [p for p in allowed if fnmatch.fnmatchcase(entry.name, p)]
        if covering:
            used.update(covering)
            continue
        problems.append(
            f".agents/composed/{entry.name}: no role selects it. Add a "
            f"composed-skill selector, or list it under "
            f"[tool.agentic-os.check-composed-skills] unselected with a reason."
        )
    problems.extend(
        f"pyproject.toml: [tool.agentic-os.check-composed-skills] unselected "
        f"pattern {pattern!r} covers no unselected source, so it exempts "
        f"nothing. Its source is gone, or a role now selects it."
        for pattern in sorted(set(allowed) - used)
    )
    return problems


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
        if not carries_content(rel, repo_root):
            continue
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
    problems = layout_problems(repo_root) + catalogue_problems(repo_root, composed)
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
