#!/usr/bin/env python3
"""Keep the brand name to its two valid spellings.

`coilyco` is lowercase everywhere it reads as a name, sentence-initial included,
the way `adidas` is. `COILYCO` is the other valid spelling, for where the name
stands alone as a title or account name. Code spans, fenced blocks, URLs and
paths are exempt, and a literal external identifier takes an allowlist entry
rather than an edit. The rule is the voice rules in AGENTS.md.
"""

from __future__ import annotations

import re
import sys
from dataclasses import dataclass
from pathlib import Path

from agentic_os.config import is_enabled, is_excluded, load_excludes, load_str_list
from agentic_os.pre_commit.tree import is_repo_content

HOOK_ID = "brand-case"
REPO_ROOT = Path.cwd()
PROSE_SUFFIXES = {".md", ".markdown", ".txt", ".njk", ".html"}

CANON = "coilyco"
# The all-caps form is valid only in a standalone title slot, which a line
# scanner cannot see, so it passes everywhere and the title-case forms still fail.
VALID_SPELLINGS = frozenset({CANON, "COILYCO"})
# Any casing, never as part of a longer word so that slugs like coilyco-bridge
# and hostnames like coilyco.ai are untouched. Valid spellings filter in scan_text.
BRAND = re.compile(r"(?<![A-Za-z0-9_-])([Cc][Oo][Ii][Ll][Yy][Cc][Oo])(?![A-Za-z0-9_-])")

FENCE = re.compile(r"^\s*(```|~~~)")
CODE_SPAN = re.compile(r"`[^`]*`")
URL = re.compile(r"<?https?://\S+|\]\([^)]*\)")


@dataclass(frozen=True)
class Violation:
    path: Path
    line: int
    column: int
    found: str

    def render(self) -> str:
        return (
            f"{self.path.as_posix()}:{self.line}:{self.column}: "
            f'"{self.found}" should be "{CANON}". The name is lowercase in prose, '
            'sentence-initial included, or "COILYCO" where it stands alone as a title.'
        )


def mask(line: str) -> str:
    """Blank out spans where the name is an identifier rather than prose."""
    for pattern in (CODE_SPAN, URL):
        line = pattern.sub(lambda m: " " * len(m.group(0)), line)
    return line


def scan_text(rel: Path, text: str, allow: frozenset[str] = frozenset()) -> list[Violation]:
    if CANON not in text.lower():
        return []
    found: list[Violation] = []
    in_fence = False
    for number, raw in enumerate(text.splitlines(), start=1):
        if FENCE.match(raw):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        for match in BRAND.finditer(mask(raw)):
            if match.group(0) in VALID_SPELLINGS:
                continue
            if any(phrase in raw for phrase in allow):
                continue
            found.append(Violation(rel, number, match.start() + 1, match.group(0)))
    return found


def scan(path: Path, rel: Path, allow: frozenset[str]) -> list[Violation]:
    try:
        text = path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return []
    return scan_text(rel, text, allow)


def main() -> int:
    if not is_enabled(HOOK_ID):
        return 0
    excludes = load_excludes(HOOK_ID)
    allow = frozenset(load_str_list(HOOK_ID, "allow", REPO_ROOT))
    violations: list[Violation] = []
    for path in sorted(REPO_ROOT.rglob("*")):
        if not path.is_file() or path.suffix.lower() not in PROSE_SUFFIXES:
            continue
        rel = path.relative_to(REPO_ROOT)
        if not is_repo_content(rel, REPO_ROOT) or is_excluded(rel, excludes):
            continue
        violations.extend(scan(path, rel, allow))
    for violation in violations:
        print(f"FAIL: {violation.render()}", file=sys.stderr)
    if violations:
        print(
            f"\n{len(violations)} brand-case violation(s). A literal external "
            "identifier that genuinely carries a capital takes an entry under "
            "[tool.agentic-os.brand-case] allow, not an edit.",
            file=sys.stderr,
        )
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
