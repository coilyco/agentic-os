#!/usr/bin/env python3
"""Generate and check the managed workspace-pointer block in a repo's AGENTS.md.

Every per-repo `AGENTS.md` opens with a one-line pointer at the workspace base.
Hand-written, it drifted into three styles and several dangled: `See ../AGENTS.md`
resolves to a non-existent `coilyco-bridge/AGENTS.md`, because the repos live in
different org dirs and are not checked out in a guaranteed sibling layout. The
canonical content already loads globally via `~/.claude/CLAUDE.md`, so the
pointer's real job is to orient a reader WITHOUT that load point (a foreign
agent, a single-repo clone, CI) - for whom only an absolute URL is true.

This module is the single source of truth for that pointer, rendered as a
marker-delimited managed block. It mirrors `generate_repo_pointer_skill`: a pure,
deterministic renderer plus a `check_drift` the validator (`check_agents_pointer`)
uses to regenerate offline and fail on any drift. The applier
(`scripts/apply-agents-pointer.py`) injects or refreshes the block in place.

Each org points at its own base, on the host appropriate to its trust tier:
  - coilyco-flight-deck/* -> the public base on GitHub (the public face).
  - coilyco-bridge/*      -> the public base on GitHub with the private
    agentic-os-kai overlay (Forgejo) layered on top, expressing that aos-pub is
    the foundation and aos-kai layers Kai-specific context over it.
The canonical base repos themselves (agentic-os, agentic-os-kai) are exempt:
a base does not point at itself. coilysiren/* stays deliberately unmanaged: it
is Kai's public personal org, outside the coilyco-* fleet, and its handful of
repos are `.agentic-os-ignore`-exempt and hand-authored (the profile repo
coilysiren/coilysiren carries a bespoke bootstrap, not a one-line pointer). So
those repos are hand-maintained per-repo rather than templated - extending
generator management over a public org would fire the drift hook across it for
    little gain.

Schema and rollout: see docs/features-agents.md.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

# Marker comments delimit the managed region so re-applies are idempotent and a
# hand-edit inside the block is caught as drift. Rest of AGENTS.md stays authored.
BEGIN = "<!-- BEGIN managed by agentic-os/scripts/apply-agents-pointer.py -->"
END = "<!-- END managed by agentic-os/scripts/apply-agents-pointer.py -->"

# Public base on its GitHub mirror (unauthenticated, the public face) and the
# private Kai overlay on canonical Forgejo. Default branch is `main` on both.
PUBLIC_BASE_URL = "https://github.com/coilyco-flight-deck/agentic-os/blob/main/AGENTS.md"
KAI_OVERLAY_URL = (
    "https://forgejo.coilysiren.me/coilyco-bridge/agentic-os-kai/src/branch/main/AGENTS.md"
)

FLIGHT_DECK = "coilyco-flight-deck"
BRIDGE = "coilyco-bridge"

# A base repo never points at itself. Keyed by (org, repo).
EXEMPT: set[tuple[str, str]] = {
    (FLIGHT_DECK, "agentic-os"),
    (BRIDGE, "agentic-os-kai"),
}

_PREFIX = "**Workspace conventions** load globally via the harness on Kai's fleet. "
_SUFFIX = " This file covers only what is specific to this repo."

# Existing managed block, matched non-greedily for replacement / drift checks.
BLOCK_RE = re.compile(re.escape(BEGIN) + r".*?" + re.escape(END), re.DOTALL)

# Legacy hand-written intro lines the applier strips before inserting the block.
# Only runs on managed consumer repos (base repos are exempt from apply).
_LEGACY_INTRO_RE = re.compile(
    r"^.*(?:"
    r"load globally via"
    r"|See `?\.\./AGENTS"
    r"|workspace-level conventions"
    r"|See the parent `?AGENTS"
    r").*$\n?",
    re.MULTILINE,
)


def render_body(org: str) -> str | None:
    """Return the pointer prose for an org, or None if the org is unmanaged."""
    if org == FLIGHT_DECK:
        return (
            f"{_PREFIX}The canonical base is "
            f"**[coilyco-flight-deck/agentic-os/AGENTS.md]({PUBLIC_BASE_URL})**."
            f"{_SUFFIX}"
        )
    if org == BRIDGE:
        return (
            f"{_PREFIX}The canonical base is the public "
            f"**[agentic-os/AGENTS.md]({PUBLIC_BASE_URL})** with the private "
            f"**[agentic-os-kai/AGENTS.md]({KAI_OVERLAY_URL})** layered on top."
            f"{_SUFFIX}"
        )
    return None


def render_block(org: str) -> str | None:
    """Return the full marker-delimited managed block, or None if unmanaged."""
    body = render_body(org)
    if body is None:
        return None
    return f"{BEGIN}\n{body}\n{END}"


def is_managed(org: str | None, repo: str | None) -> bool:
    """True if this (org, repo) gets a managed pointer block."""
    if org is None:
        return False
    if (org, repo) in EXEMPT:
        return False
    return render_body(org) is not None


def apply_to_text(text: str, org: str) -> str | None:
    """Return AGENTS.md text with the managed block inserted or refreshed.

    Idempotent: strips any prior managed block and known legacy intro lines,
    then inserts the fresh block right after the first H1. Returns None for an
    unmanaged org (caller should leave the file untouched).
    """
    block = render_block(org)
    if block is None:
        return None
    text = BLOCK_RE.sub("", text)
    text = _LEGACY_INTRO_RE.sub("", text)
    text = _insert_after_h1(text, block)
    return _normalize_blank_lines(text)


def _insert_after_h1(text: str, block: str) -> str:
    lines = text.splitlines()
    for i, line in enumerate(lines):
        if line.startswith("# "):
            lines[i + 1 : i + 1] = ["", block]
            return "\n".join(lines) + ("\n" if text.endswith("\n") else "")
    # No H1: place the block at the very top.
    return f"{block}\n\n{text}"


def _normalize_blank_lines(text: str) -> str:
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text


def check_drift(text: str, org: str) -> list[str]:
    """Return human-readable defects for a managed AGENTS.md (offline, no net).

    The block must be present and byte-identical to `render_block(org)`. A
    surviving legacy intro line alongside the block is also flagged, so an
    apply that half-migrated does not pass.
    """
    block = render_block(org)
    if block is None:
        return []
    found = BLOCK_RE.search(text)
    if not found:
        return [
            "AGENTS.md: missing the managed workspace-pointer block. "
            "Generate it with scripts/apply-agents-pointer.py."
        ]
    problems: list[str] = []
    if found.group(0) != block:
        problems.append(
            "AGENTS.md: managed workspace-pointer block drifted from generator "
            "output. This block is auto-generated; do not hand-edit. "
            "Regenerate with scripts/apply-agents-pointer.py."
        )
    leftover = _LEGACY_INTRO_RE.search(BLOCK_RE.sub("", text))
    if leftover:
        problems.append(
            "AGENTS.md: a legacy pointer line survives alongside the managed "
            f"block: {leftover.group(0).strip()!r}. Re-run the applier to remove it."
        )
    return problems


_REMOTE_RE = re.compile(
    r"(?:github\.com[:/]|forgejo\.coilysiren\.me/)(?P<org>[^/]+)/(?P<repo>[^/.\s]+)"
)


def detect_org_repo(repo_root: Path) -> tuple[str | None, str | None]:
    """Derive (org, repo) from the repo's git remotes. (None, None) if unknown."""
    try:
        proc = subprocess.run(
            ["git", "remote", "-v"],
            cwd=repo_root,
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError:
        return None, None
    for m in _REMOTE_RE.finditer(proc.stdout):
        return m.group("org"), m.group("repo")
    return None, None


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="generate-agents-pointer",
        description="Render the managed AGENTS.md workspace-pointer block for an org.",
    )
    parser.add_argument(
        "--org",
        help="Org slug (coilyco-flight-deck / coilyco-bridge). Default: detect from cwd's remotes.",
    )
    parser.add_argument(
        "--repo-root",
        default=".",
        help="Repo root to detect the org from (default: cwd).",
    )
    args = parser.parse_args(argv)

    org = args.org
    if org is None:
        org, _ = detect_org_repo(Path(args.repo_root))
    block = render_block(org) if org else None
    if block is None:
        print(
            f"generate-agents-pointer: org {org!r} is unmanaged; nothing to render.",
            file=sys.stderr,
        )
        return 1
    print(block)
    return 0


if __name__ == "__main__":
    sys.exit(main())
