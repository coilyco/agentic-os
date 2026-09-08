#!/usr/bin/env python3
"""Generate and check the managed git-workflow block in a repo's AGENTS.md.

Every repo's landing lane is declared once, as `ward.workflow` in the AGENTS.md
frontmatter, and was then restated by hand in a one-line `**Git workflow** -`
stamp under `## Agent rules`. Hand-stamped, it said what the lane was without
ever saying that the lane is a standing authorization, so an agent reading it
still treated a commit, a branch, a push, or a pull request as an action worth
stopping to ask about. A turn that stops there ends with the work stranded in a
dirty worktree, which is the loss vector agentic-os#1150 records.

This module is the single source of truth for that stamp, rendered as a
marker-delimited managed block. It mirrors `generate_agents_pointer`: a pure,
deterministic renderer plus a `check_drift` the validator (`check_git_workflow`)
uses to regenerate offline and fail on any drift. The applier
(`scripts/apply-git-workflow.py`) injects or refreshes the block in place.

The block names the one lane the fleet runs, `pull-request-and-merge`, says which
lane this repo declares, and states the pre-authorization in MUST / ALWAYS / NEVER
terms, so an agent that has read it never needs to ask whether committing or
pushing is allowed here. The two genuine walls, `--no-verify` and force-push, stay
closed in the same breath, so the block reads as a boundary rather than a blanket.

`merge-remote-main` was retired here. It was the lane that let an agent push the
default branch directly, and pushing straight to `main` ended fleet-wide. Removing
the slug from `LANES` is what makes it unrenderable: a repo declaring it now reads
as undeclared and gets the guarded `pull-request` shape rather than a direct push.
One repo still declares it and keeps it deliberately, `coilysiren/coilysiren`. That
one is GitHub-canonical, carries `.agentic-os-ignore`, wires no catalog hooks, and
holds no managed block, so nothing here renders or validates its frontmatter.

Every slug names what the AGENT does, and the block says so outright because
the first two drafts of this generator got it backwards. `pull-request-and-merge`
carries the merge because the agent that authored the code merges its own pull
request, which makes it the fully autonomous lane rather than the gated one.
`pull-request` drops `-and-merge` because the author stops at the pull request
and the director merge lane takes over. Ward's `agent_director_merge` currently
gates director merges on the opposite slug, so its behavior contradicts this
until someone reconciles it.

Every repo with a root AGENTS.md gets the block, org-agnostic and with no base
repo exempt. A lane binds the agent in the canonical base exactly as it binds
one in a consumer, unlike the workspace pointer, which a base does not owe
itself. A repo that declares no lane renders the `pull-request` variant, the
one lane that neither pushes `main` nor merges, rather than guessing at an
authority the repo never granted.

`--print-lane` exists because `scripts/pr-guard-pre-push.sh` is bash and
needs the same answer this module already owns, rather than a second
frontmatter parser of its own (agentic-os#1321).

Schema and rollout: see docs/features-agents.md.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

from agentic_os.frontmatter import split_frontmatter

# Marker comments delimit the managed region so re-applies are idempotent and a
# hand-edit inside the block is caught as drift. Rest of AGENTS.md stays authored.
BEGIN = "<!-- BEGIN managed by agentic-os/scripts/apply-git-workflow.py -->"
END = "<!-- END managed by agentic-os/scripts/apply-git-workflow.py -->"

MERGE_MAIN = "merge-remote-main"
PR_AND_MERGE = "pull-request-and-merge"
PULL_REQUEST = "pull-request"
BRANCH_ONLY = "remote-branch-only"

# MERGE_MAIN is deliberately absent: retired, and unrenderable by construction.
LANES = (PULL_REQUEST, PR_AND_MERGE, BRANCH_ONLY)

_DECLARED = "declared as `ward.workflow` in this file's frontmatter"

# Lead paragraph per lane. `None` keys the undeclared variant, which never
# grants a direct push to `main` on a guess.
_LEAD: dict[str | None, str] = {
    PR_AND_MERGE: (
        f"**This repo runs the `{PR_AND_MERGE}` lane**, {_DECLARED}. The agent "
        "commits to a task branch, pushes it, opens a Forgejo pull request, "
        "and **merges that pull request itself** once it is green. The author "
        "of the code is the one who merges it. Opening the pull request is a "
        "step, never the stopping point."
    ),
    PULL_REQUEST: (
        f"**This repo runs the `{PULL_REQUEST}` lane**, {_DECLARED}. The agent "
        "commits to a task branch, pushes it, opens a Forgejo pull request, "
        "and stops there. The author does not merge on this lane. The director "
        "merge lane takes it from the pull request onward."
    ),
    BRANCH_ONLY: (
        f"**This repo runs the `{BRANCH_ONLY}` lane**, {_DECLARED}. The agent "
        "commits to a task branch and pushes it, then stops. This is the one "
        "lane that owes no pull request."
    ),
    None: (
        "**This repo declares no `ward.workflow` lane.** Until it declares one "
        f"in this file's frontmatter, the agent MUST work the `{PULL_REQUEST}` "
        "shape: task branch, push, pull request, then stop. No direct push to "
        "`main`, and no agent merge."
    ),
}

_BODY = f"""The fleet runs one lane, and it authorizes the agent end to end. Pushing straight to `main` is over: `{MERGE_MAIN}` is retired, so no repo can declare its way back to one.

* `{PR_AND_MERGE}` - the agent commits to a task branch, pushes it, opens a pull request, and merges that pull request itself once it is green.

**Every lane slug names what the AGENT does, never what someone else does.** `{PR_AND_MERGE}` carries the merge because the agent that authored the code merges its own pull request. `{PULL_REQUEST}` drops `-and-merge` because the author stops at the pull request and the director merge lane takes over. Reading `{PR_AND_MERGE}` as "someone else merges it later" inverts the two and leaves finished work sitting unmerged.

**These actions are pre-authorized on every lane, and the agent MUST take them without asking first.** Committing, creating a branch, pushing a branch, pushing the lane's own destination, and opening a pull request are ordinary reversible work, not the destructive wall that earns a question. Stopping to ask is how a turn ends with the work stranded in a dirty worktree.

* **ALWAYS commit** in-scope work and **ALWAYS push** it to the canonical remote before pausing, reporting a checkpoint, handing off, or ending a turn. A local-only commit is not a checkpoint.
* **ALWAYS open the pull request** in the same turn as the branch's first push, on every lane except `{BRANCH_ONLY}`. A pushed branch with no pull request is litter nobody reviews.
* **NEVER `--no-verify`** and **NEVER force-push**. Those two are the real walls, and they stay closed.
* **ALWAYS merge your own pull request on `{PR_AND_MERGE}`**, in the same turn, as soon as it is green. Reporting it as open and awaiting someone is the failure this lane exists to prevent.
* **NEVER merge on `{PULL_REQUEST}` or `{BRANCH_ONLY}`.** Those two stop where they stop, and the director merge lane carries a `{PULL_REQUEST}` from there."""

# Existing managed block, matched non-greedily for replacement / drift checks.
BLOCK_RE = re.compile(re.escape(BEGIN) + r".*?" + re.escape(END), re.DOTALL)

# The legacy hand-written one-line stamp the applier strips before inserting.
_LEGACY_STAMP_RE = re.compile(r"^\*\*Git workflow\*\*[^\n]*$\n?", re.MULTILINE)

# Blanks, not `\s`: a newline-eating `\s*$` walks the anchor off its own line
# and swallows the blank line the inserted block needs after it.
_AGENT_RULES_RE = re.compile(r"^##[ \t]+Agent rules[ \t]*$", re.MULTILINE)


def normalize_lane(lane: object) -> str | None:
    """Return a known lane slug, or None for absent, unknown, or malformed."""
    if isinstance(lane, str) and lane.strip() in LANES:
        return lane.strip()
    return None


def detect_lane(text: str) -> str | None:
    """Read `ward.workflow` out of an AGENTS.md's YAML frontmatter."""
    metadata, _ = split_frontmatter(text)
    ward = metadata.get("ward")
    if not isinstance(ward, dict):
        return None
    return normalize_lane(ward.get("workflow"))


def render_body(lane: str | None) -> str:
    """Return the block prose for a lane. An unknown lane renders undeclared."""
    return f"### Git workflow\n\n{_LEAD[normalize_lane(lane)]}\n\n{_BODY}"


def render_block(lane: str | None) -> str:
    """Return the full marker-delimited managed block for a lane."""
    return f"{BEGIN}\n{render_body(lane)}\n{END}"


def apply_to_text(text: str) -> str:
    """Return AGENTS.md text with the managed block inserted or refreshed.

    Idempotent: strips any prior managed block and the legacy one-line stamp,
    then inserts the fresh block under `## Agent rules`. The lane is read from
    the file's own frontmatter, so the applier needs no argument beyond text.
    """
    lane = detect_lane(text)
    text = BLOCK_RE.sub("", text)
    text = _LEGACY_STAMP_RE.sub("", text)
    return _normalize_blank_lines(_insert(text, render_block(lane)))


def _insert(text: str, block: str) -> str:
    """Place the block under `## Agent rules`, or at the end without one."""
    found = _AGENT_RULES_RE.search(text)
    if found is None:
        return text.rstrip("\n") + f"\n\n{block}\n"
    line_end = text.find("\n", found.end())
    if line_end == -1:
        return text.rstrip("\n") + f"\n\n{block}\n"
    at = line_end + 1
    return text[:at] + f"\n{block}\n" + text[at:]


def _normalize_blank_lines(text: str) -> str:
    return re.sub(r"\n{3,}", "\n\n", text)


def check_drift(text: str) -> list[str]:
    """Return human-readable defects for an AGENTS.md (offline, no net).

    The block must be present and byte-identical to `render_block(lane)` for the
    lane the same file declares. A surviving legacy stamp beside the block is
    also flagged, so an apply that half-migrated does not pass.
    """
    block = render_block(detect_lane(text))
    found = BLOCK_RE.search(text)
    if not found:
        return [
            "AGENTS.md: missing the managed git-workflow block. "
            "Generate it with scripts/apply-git-workflow.py."
        ]
    problems: list[str] = []
    if found.group(0) != block:
        problems.append(
            "AGENTS.md: managed git-workflow block drifted from generator "
            "output, or no longer matches the lane this file declares. This "
            "block is auto-generated; do not hand-edit. Regenerate with "
            "scripts/apply-git-workflow.py."
        )
    leftover = _LEGACY_STAMP_RE.search(BLOCK_RE.sub("", text))
    if leftover:
        problems.append(
            "AGENTS.md: a legacy one-line git-workflow stamp survives beside "
            f"the managed block: {leftover.group(0).strip()!r}. Re-run the "
            "applier to remove it."
        )
    return problems


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="generate-git-workflow",
        description="Render the managed AGENTS.md git-workflow block for a lane.",
    )
    parser.add_argument(
        "--lane",
        choices=LANES,
        help="Lane slug. Default: read ward.workflow from --agents-md.",
    )
    parser.add_argument(
        "--agents-md",
        default="AGENTS.md",
        help="AGENTS.md to read the lane from (default: ./AGENTS.md).",
    )
    parser.add_argument(
        "--print-lane",
        action="store_true",
        help="Print the resolved lane slug instead of the block, and print "
        "nothing at all when the repo declares none.",
    )
    args = parser.parse_args(argv)

    lane = args.lane
    if lane is None:
        path = Path(args.agents_md)
        if path.is_file():
            lane = detect_lane(path.read_text(encoding="utf-8", errors="replace"))
        elif not args.print_lane:
            print(
                f"generate-git-workflow: no {args.agents_md} to read a lane "
                "from; rendering the undeclared variant.",
                file=sys.stderr,
            )

    # Absence prints as absence: a fallback slug would hand pr-guard an
    # authority the repo never granted. See this module's docstring.
    if args.print_lane:
        resolved = normalize_lane(lane)
        if resolved is not None:
            print(resolved)
        return 0

    print(render_block(lane))
    return 0


if __name__ == "__main__":
    sys.exit(main())
