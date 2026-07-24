#!/usr/bin/env python3
"""Enforce repo documentation placement and size.

This module is the single source of truth for Markdown size caps across the
agentic-os ecosystem. Docs (AGENTS.md, SKILL.md.template, handbook.md, etc.)
should point here by reference rather than restating numbers, so the caps
can never drift between code and prose.

Markdown documentation may live only in:
    1. the repo root, with a small universal filename allow-list;
    2. docs/*.md, with no docs subdirectories;
    3. skill folders (.agents/skills, .agents/composed, .claude/skills, or
       skills), which may carry support subdirs. No nested skill entry point
       may hide below the top-level skill dir;
    4. anywhere under an `examples/` directory at any depth, any .md filename;
    5. a co-located module README.md, but only in one of two tightly-capped
       shapes (see below). Any other co-located Markdown is still a violation.

Module README.md shapes
-----------------------
A README.md anywhere below the root (outside docs/, skill, examples) is the
one co-located doc the layout permits, because deep docs are invisible to
anyone not running `rg`. To keep substance in the central, browsable docs/
index rather than scattered through the tree, the co-located README may only
be one of two tightly-capped shapes - the cap is the forcing function, since
nothing worth hiding fits in 3 lines:

    * outpost  - a pure redirect into docs/. <= 3 non-blank lines: a heading,
      an optional one-sentence summary, and exactly one link to a single
      docs/*.md file. Reciprocal: that docs file must link back to this exact
      README path (file-to-file). One docs file may be the target of many
      outposts (each gets its own back-link); one outpost points at one doc.
    * homestead - self-contained signage that points nowhere. <= 3 non-blank
      lines (heading + up to 2 content lines), no docs/ pointer.

Both cap prose lines at README_MAX_PROSE_CHARS. The pointer line of an
outpost is exempt from that cap - a deep relative path is not prose. Blank
lines never count toward the line ceiling. The discriminator is simple: a
README that links a docs/*.md file is an outpost, otherwise it is a
homestead.

Most Markdown shares one size cap: MAX_MARKDOWN_LINES / MAX_MARKDOWN_CHARS.
SKILL.md and COMPOSED.md are not special. CLAUDE.md is expected to be a
one-line `@AGENTS.md` pointer.

The root README.md and AGENTS.md carry each project's living overview (intro
and operating doctrine), so both get more room than ordinary Markdown.
README.md uses TRIFECTA_MAX_LINES / TRIFECTA_MAX_CHARS. AGENTS.md gets a larger
default because universal person and operating context must fire eagerly while
selective capability detail moves into ordinary and composed skills.
docs/FEATURES.md is a coarse inventory of major shipped capabilities, so it
gets the tighter FEATURES_MAX_LINES / FEATURES_MAX_CHARS cap. Bounded, not
infinite: durable detail still belongs in docs/*.md. README.md only at repo
root; a co-located module README stays on the tight outpost / homestead shape.

AGENTS.md may opt past its default cap, per-repo, via config keys
`agents_md_max_lines` / `agents_md_max_chars` under the documentation-layout
hook section. Repos that don't set them get the shared AGENTS.md default.
The canonical agentic-os-kai AGENTS.md is loader-bound (read on every session)
and holds universal-fire doctrine that can't split into docs/*.md without
losing unconditional firing, so that repo opts higher.

The root README.md gets the same per-repo opt-up, via `readme_max_lines` /
`readme_max_chars`. It is the launch-grade front page a cold visitor (human or
agent) reads to decide in under a minute whether the tool is for them, so a
release repo may need room to say who it's for and what it requires, show a
denial, and route into docs/ - more than the trifecta default gives. Repos
that don't set the keys keep the trifecta cap. Only the root README.md opts up;
a co-located module README stays on the tight outpost / homestead shape.
"""
from __future__ import annotations

import re
import sys
from pathlib import Path, PurePosixPath

from agentic_os.config import (
    get_int_option,
    is_enabled,
    is_excluded,
    load_excludes,
)

REPO_ROOT = Path.cwd()
HOOK_ID = "documentation-layout"
MAX_MARKDOWN_LINES = 80
MAX_MARKDOWN_CHARS = 4_000

# Co-located module README.md caps (outpost / homestead shapes; see docstring).
# Non-blank lines per README, and prose chars per line (pointer line exempt).
README_MAX_LINES = 3
README_MAX_PROSE_CHARS = 90

# Inline Markdown link: [text](target). Reference-style links are not matched -
# outposts and back-links use inline links.
MD_LINK_RE = re.compile(r"\[[^\]]*\]\(([^)\s]+)\)")

# The broad overview files get more room than a focused docs/ inventory page -
# bounded, not infinite. See the module docstring.
TRIFECTA_MAX_LINES = 160
TRIFECTA_MAX_CHARS = 12_500

# FEATURES.md is the inventory-shaped exception: tighter than the overview
# files so it stays a major-capability index, not a changelog.
FEATURES_MAX_LINES = 80
FEATURES_MAX_CHARS = 4_000

# AGENTS.md carries universal-fire context after selective capability detail
# moves into ordinary and composed skills. Repos may opt higher via config.
AGENTS_DEFAULT_MAX_LINES = TRIFECTA_MAX_LINES * 2
AGENTS_DEFAULT_MAX_CHARS = TRIFECTA_MAX_CHARS * 2

# The root README.md - the launch-grade front page - defaults to the trifecta
# cap and opts higher per-repo via readme_max_lines / readme_max_chars.
README_DEFAULT_MAX_LINES = TRIFECTA_MAX_LINES
README_DEFAULT_MAX_CHARS = TRIFECTA_MAX_CHARS

# Verbatim upstream files; exempt from size cap, matched by basename.
SIZE_CAP_EXEMPT_BASENAMES = {
    "CODE_OF_CONDUCT.md",
}

ROOT_MARKDOWN_ALLOWLIST = {
    "AGENTS.md",
    # agent-compose's disjoint root source. Has its own size/dedup hooks
    # (check-agent-compose-*), and no AGENTS.md/CLAUDE.md cascade loads it.
    "AGENTS.COMPOSE.md",
    "CLAUDE.md",
    "CODE_OF_CONDUCT.md",
    "CODE-REVIEW.md",
    "CONTRIBUTING.md",
    "GOVERNANCE.md",
    "LICENSE.md",
    "README.md",
    "SECURITY.md",
    "SUPPORT.md",
}

# agent-compose per-harness override AGENTS.<harness>.md sits beside its base.
# Lowercase token mirrors a harness slug, so uppercase AGENTS.COMPOSE.md misses.
HARNESS_OVERRIDE_RE = re.compile(r"^AGENTS\.[a-z][a-z0-9-]*\.md$")


def is_harness_override(name: str) -> bool:
    return bool(HARNESS_OVERRIDE_RE.match(name))


SKIP_DIR_NAMES = {
    ".claude",
    ".git",
    ".mypy_cache",
    ".pytest_cache",
    ".ruff_cache",
    ".terraform",
    ".tox",
    ".venv",
    "__pycache__",
    "build",
    "dist",
    "node_modules",
    "target",
    "vendor",
}

SKILL_PATHS = (
    (".agents", "skills"),
    (".agents", "composed"),
    (".claude", "skills"),
    ("skills",),
)


def should_skip(path: Path) -> bool:
    return any(part in SKIP_DIR_NAMES for part in path.parts)


def is_under_skill_path(rel: Path) -> bool:
    parts = rel.parts
    for skill_parts in SKILL_PATHS:
        n = len(skill_parts)
        if len(parts) >= n and parts[:n] == skill_parts:
            return True
    return False


def markdown_files() -> list[Path]:
    excludes = load_excludes(HOOK_ID)
    out: list[Path] = []
    for path in REPO_ROOT.rglob("*.md"):
        rel = path.relative_to(REPO_ROOT)
        if should_skip(rel):
            continue
        if is_excluded(rel, excludes):
            continue
        out.append(rel)
    return sorted(out)


def check_docs_flatness() -> list[str]:
    docs = REPO_ROOT / "docs"
    if not docs.is_dir():
        return []
    excludes = load_excludes(HOOK_ID)
    violations: list[str] = []
    for path in sorted(docs.rglob("*")):
        rel = path.relative_to(REPO_ROOT)
        if should_skip(rel):
            continue
        if is_excluded(rel, excludes):
            continue
        if path.is_dir() and path != docs:
            violations.append(
                f"{rel.as_posix()}: docs/ must stay flat. Use filename prefixes instead "
                f"of docs subdirectories."
            )
    return violations


def is_under_examples(rel: Path) -> bool:
    # Go/Rust examples/<name>/... is idiomatic at any depth and may contain
    # .md files of any name, not just README.md.
    return "examples" in rel.parts


def _normalize_link_target(target: str, source_rel: Path) -> str | None:
    """Resolve a Markdown link to a repo-root-relative POSIX path.

    `source_rel` is the repo-relative path of the file the link lives in.
    Anchors/queries are stripped, `..`/`.` are collapsed, and external or
    in-page links (http(s):, mailto:, bare #anchor) return None. The result is
    suitable for comparing two links for reciprocity regardless of whether
    they were written root-absolute (/docs/x.md) or relative (../docs/x.md).
    """
    target = target.strip().split("#", 1)[0].split("?", 1)[0]
    if not target:
        return None
    if target.startswith(("http://", "https://", "mailto:", "//")):
        return None
    if target.startswith("/"):
        raw = PurePosixPath(target.lstrip("/"))
    else:
        raw = PurePosixPath(source_rel.parent.as_posix()) / target
    parts: list[str] = []
    for part in raw.parts:
        if part in (".", ""):
            continue
        if part == "..":
            if parts:
                parts.pop()
            continue
        parts.append(part)
    return "/".join(parts) if parts else None


def _is_docs_md(norm: str) -> bool:
    parts = norm.split("/")
    return len(parts) == 2 and parts[0] == "docs" and parts[1].endswith(".md")


def _check_reciprocity(
    readme_rel: Path, docs_target: str, repo_root: Path
) -> list[str]:
    """The docs file an outpost points at must link back to that outpost."""
    docs_path = repo_root / docs_target
    if not docs_path.is_file():
        return [
            f"{readme_rel.as_posix()}: outpost points at {docs_target}, which "
            f"does not exist."
        ]
    docs_rel = Path(docs_target)
    readme_posix = readme_rel.as_posix()
    text = docs_path.read_text(encoding="utf-8", errors="replace")
    for match in MD_LINK_RE.finditer(text):
        if _normalize_link_target(match.group(1), docs_rel) == readme_posix:
            return []
    return [
        f"{readme_posix}: outpost <-> {docs_target} link is not reciprocal. "
        f"{docs_target} must link back to {readme_posix} (file-to-file)."
    ]


def validate_module_readme(rel: Path, repo_root: Path) -> list[str]:
    """Validate a co-located README.md as an outpost or a homestead.

    Outpost: <= 3 non-blank lines, exactly one docs/*.md link, reciprocal.
    Homestead: <= 3 non-blank lines, no docs/*.md link. Prose lines (every
    non-blank line except an outpost's pointer line) cap at 90 chars; blank
    lines do not count toward the line ceiling.
    """
    path = repo_root / rel
    text = path.read_text(encoding="utf-8", errors="replace")
    nonblank = [ln.strip() for ln in text.splitlines() if ln.strip()]
    rel_posix = rel.as_posix()

    docs_targets: list[str] = []
    pointer_lines: set[int] = set()
    for i, line in enumerate(nonblank):
        for match in MD_LINK_RE.finditer(line):
            norm = _normalize_link_target(match.group(1), rel)
            if norm and _is_docs_md(norm):
                docs_targets.append(norm)
                pointer_lines.add(i)

    shape = "outpost" if docs_targets else "homestead"
    violations: list[str] = []

    if len(nonblank) > README_MAX_LINES:
        violations.append(
            f"{rel_posix}: module README ({shape}) has {len(nonblank)} "
            f"non-blank lines, max {README_MAX_LINES}. Move the body into a "
            f"docs/*.md file and leave a pointer, or trim to signage."
        )

    if nonblank and not nonblank[0].startswith("#"):
        violations.append(
            f"{rel_posix}: module README ({shape}) first line must be a "
            f"Markdown heading."
        )

    for i, line in enumerate(nonblank):
        if i in pointer_lines:
            continue
        if len(line) > README_MAX_PROSE_CHARS:
            violations.append(
                f"{rel_posix}: prose line {i + 1} is {len(line)} chars, max "
                f"{README_MAX_PROSE_CHARS}."
            )

    if docs_targets:
        distinct = sorted(set(docs_targets))
        if len(distinct) > 1:
            violations.append(
                f"{rel_posix}: outpost points at {len(distinct)} docs files "
                f"({', '.join(distinct)}); it may point at exactly one."
            )
        for target in distinct:
            violations += _check_reciprocity(rel, target, repo_root)

    return violations


def check_markdown_locations() -> list[str]:
    violations: list[str] = []
    for rel in markdown_files():
        if len(rel.parts) == 1:
            if rel.name not in ROOT_MARKDOWN_ALLOWLIST and not is_harness_override(
                rel.name
            ):
                allowed = ", ".join(sorted(ROOT_MARKDOWN_ALLOWLIST))
                violations.append(
                    f"{rel.as_posix()}: top-level Markdown filename is not allowed. "
                    f"Allowed root Markdown files: {allowed}. Move one-off "
                    f"docs into docs/."
                )
            continue
        if rel.parts[0] == "docs" and len(rel.parts) == 2:
            continue
        if is_under_skill_path(rel):
            continue
        if is_under_examples(rel):
            continue
        if rel.name == "README.md":
            # The one co-located doc the layout allows, held to the tight
            # outpost / homestead shape instead of banned outright.
            violations += validate_module_readme(rel, REPO_ROOT)
            continue
        violations.append(
            f"{rel.as_posix()}: Markdown files may live only at repo root, docs/*.md, "
            f"a skill folder, or a capped module README.md."
        )
    return violations


def check_skill_flatness(repo_root: Path | None = None) -> list[str]:
    """Flag nested sub-skills, not support material.

    The skill loader only sees top-level skill dirs. SKILL.md or COMPOSED.md
    nested below the top level is invisible and must move up. Support subdirs
    are fine because the rule targets hidden sub-skills.
    """
    root = repo_root or REPO_ROOT
    excludes = load_excludes(HOOK_ID, root)
    violations: list[str] = []
    for skill_parts in SKILL_PATHS:
        skill_root = root.joinpath(*skill_parts)
        if not skill_root.is_dir():
            continue
        for skill_dir in sorted(skill_root.iterdir()):
            if not skill_dir.is_dir():
                continue
            if should_skip(skill_dir.relative_to(root)):
                continue
            for entrypoint in ("SKILL.md", "COMPOSED.md"):
                for nested in sorted(skill_dir.rglob(entrypoint)):
                    if nested.parent == skill_dir:
                        continue
                    rel = nested.relative_to(root)
                    if should_skip(rel):
                        continue
                    if is_excluded(rel, excludes):
                        continue
                    violations.append(
                        f"{rel.as_posix()}: nested {entrypoint} must not hide below "
                        f"the top-level skill dir. Move this sub-skill beside the others."
                    )
    return violations


def caps_for(rel: Path) -> tuple[int, int]:
    if rel.name == "AGENTS.md":
        max_lines = get_int_option(
            HOOK_ID, "agents_md_max_lines", AGENTS_DEFAULT_MAX_LINES
        )
        max_chars = get_int_option(
            HOOK_ID, "agents_md_max_chars", AGENTS_DEFAULT_MAX_CHARS
        )
        return max_lines, max_chars
    if rel.as_posix() == "README.md":
        max_lines = get_int_option(
            HOOK_ID, "readme_max_lines", README_DEFAULT_MAX_LINES
        )
        max_chars = get_int_option(
            HOOK_ID, "readme_max_chars", README_DEFAULT_MAX_CHARS
        )
        return max_lines, max_chars
    if rel.as_posix() == "docs/FEATURES.md":
        return FEATURES_MAX_LINES, FEATURES_MAX_CHARS
    return MAX_MARKDOWN_LINES, MAX_MARKDOWN_CHARS


def check_markdown_sizes() -> list[str]:
    violations: list[str] = []
    for rel in markdown_files():
        if rel.name in SIZE_CAP_EXEMPT_BASENAMES:
            continue
        path = REPO_ROOT / rel
        text = path.read_text(encoding="utf-8", errors="replace")
        n_lines = len(text.splitlines())
        n_chars = len(text)
        max_lines, max_chars = caps_for(rel)
        if n_lines > max_lines:
            violations.append(
                f"{rel.as_posix()}: {n_lines} lines exceeds the {max_lines}-line "
                f"cap. Split large docs into smaller docs/*.md files."
            )
        if n_chars > max_chars:
            violations.append(
                f"{rel.as_posix()}: {n_chars} chars exceeds the {max_chars}-char "
                f"cap. Split large docs into smaller docs/*.md files."
            )
    return violations


def main() -> int:
    if not is_enabled(HOOK_ID):
        print(f"{HOOK_ID}: disabled by repo config")
        return 0
    violations = (
        check_docs_flatness()
        + check_markdown_locations()
        + check_markdown_sizes()
        + check_skill_flatness()
    )
    if not violations:
        print("documentation-layout check: OK")
        return 0
    for violation in violations:
        sys.stderr.write(f"FAIL: {violation}\n")
    sys.stderr.write(f"\n{len(violations)} documentation layout violation(s).\n")
    return 1


if __name__ == "__main__":
    sys.exit(main())
