#!/usr/bin/env python3
"""Keep the brand name lowercase in prose.

`coilyco` is set lowercase everywhere it is read as a name, including at the
start of a sentence, the way `adidas` and `ebay` are. Capitalising it is the
single most common way the name drifts, because ordinary sentence-casing does
it automatically and nobody notices.

Code spans, fenced blocks, URLs, and paths are left alone: a slug, a hostname
and an identifier are not prose, and the name is already lowercase there. Some
capitalised strings are literal external identifiers rather than the brand - a
certificate subject, an MCP connector's display name - and those take an
allowlist rather than an edit. See docs/brand-case.md.
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
# Any casing but the canonical one, and never as part of a longer word so that
# slugs like coilyco-bridge and hostnames like coilyco.ai are untouched.
WRONG_CASE = re.compile(r"(?<![A-Za-z0-9_-])(?!coilyco)([Cc][Oo][Ii][Ll][Yy][Cc][Oo])(?![A-Za-z0-9_-])")

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
            "sentence-initial included."
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
        for match in WRONG_CASE.finditer(mask(raw)):
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
