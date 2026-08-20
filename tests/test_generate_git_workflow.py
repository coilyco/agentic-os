import pytest

from agentic_os.generators.generate_git_workflow import (
    BEGIN,
    BRANCH_ONLY,
    END,
    LANES,
    MERGE_MAIN,
    PR_AND_MERGE,
    PULL_REQUEST,
    apply_to_text,
    check_drift,
    detect_lane,
    normalize_lane,
    render_block,
    render_body,
)

AGENTS = """---
ward:
  workflow: {lane}
---
# Agent instructions

Intro prose.

## Agent rules

**Git workflow** - `{lane}`, declared as `ward.workflow` in this file's frontmatter.

### Pronouns

Kai is she/her.
"""

NO_FRONTMATTER = """# Agent instructions

## Agent rules

### Pronouns

Kai is she/her.
"""


def test_detect_lane_reads_ward_workflow():
    assert detect_lane(AGENTS.format(lane=MERGE_MAIN)) == MERGE_MAIN
    assert detect_lane(AGENTS.format(lane=PR_AND_MERGE)) == PR_AND_MERGE


def test_detect_lane_is_none_without_a_declaration():
    assert detect_lane(NO_FRONTMATTER) is None
    assert detect_lane("---\nward: not-a-mapping\n---\n# x\n") is None
    assert detect_lane("---\nward:\n  workflow: invented-lane\n---\n# x\n") is None


def test_normalize_lane_rejects_non_strings_and_unknown_slugs():
    assert normalize_lane(None) is None
    assert normalize_lane(42) is None
    assert normalize_lane("push-whatever") is None
    assert normalize_lane("  merge-remote-main  ") == MERGE_MAIN


@pytest.mark.parametrize("lane", LANES)
def test_every_lane_renders_a_marker_delimited_block(lane):
    block = render_block(lane)
    assert block.startswith(BEGIN)
    assert block.endswith(END)
    assert f"**This repo runs the `{lane}` lane**" in block


def test_undeclared_lane_never_grants_a_direct_push_to_main():
    body = render_body(None)
    assert "declares no `ward.workflow` lane" in body
    assert "never a direct push to `main`" in body
    # An unknown slug is undeclared, not a lane of its own.
    assert render_body("invented-lane") == body


def test_block_details_both_fleet_lanes_whichever_one_is_active():
    for lane in (MERGE_MAIN, PR_AND_MERGE, None):
        block = render_block(lane)
        assert f"* `{MERGE_MAIN}` -" in block
        assert f"* `{PR_AND_MERGE}` -" in block


def test_block_states_the_authorization_in_strong_terms():
    block = render_block(PR_AND_MERGE)
    assert "MUST take them without asking first" in block
    assert "**ALWAYS commit**" in block
    assert "**ALWAYS push**" in block
    assert "**ALWAYS open the pull request**" in block
    assert "**NEVER `--no-verify`**" in block
    assert "**NEVER force-push**" in block


def test_branch_only_is_the_one_lane_excused_from_the_pull_request():
    block = render_block(BRANCH_ONLY)
    assert "owes no pull request" in block
    assert f"on every lane except `{BRANCH_ONLY}`" in block


def test_apply_replaces_the_legacy_stamp_under_agent_rules():
    text = AGENTS.format(lane=PULL_REQUEST)
    out = apply_to_text(text)
    assert "**Git workflow** - `pull-request`" not in out
    assert out.index(BEGIN) > out.index("## Agent rules")
    assert out.index(END) < out.index("### Pronouns")
    assert "<!-- END managed by agentic-os/scripts/apply-git-workflow.py -->\n\n### Pronouns" in out


def test_apply_renders_the_lane_the_file_declares():
    out = apply_to_text(AGENTS.format(lane=MERGE_MAIN))
    assert f"**This repo runs the `{MERGE_MAIN}` lane**" in out
    assert check_drift(out) == []


def test_apply_is_idempotent():
    once = apply_to_text(AGENTS.format(lane=PR_AND_MERGE))
    assert apply_to_text(once) == once
    assert once.count(BEGIN) == 1


def test_apply_refreshes_a_block_left_on_a_stale_lane():
    stale = apply_to_text(AGENTS.format(lane=MERGE_MAIN))
    relaned = stale.replace(f"workflow: {MERGE_MAIN}", f"workflow: {PR_AND_MERGE}", 1)
    assert check_drift(relaned)  # the block now contradicts the declared lane
    assert check_drift(apply_to_text(relaned)) == []


def test_apply_appends_when_the_file_has_no_agent_rules_heading():
    out = apply_to_text("# Agent instructions\n\nIntro prose.\n")
    assert out.rstrip("\n").endswith(END)
    assert check_drift(out) == []


def test_apply_reaches_a_file_with_no_frontmatter():
    out = apply_to_text(NO_FRONTMATTER)
    assert "declares no `ward.workflow` lane" in out
    assert check_drift(out) == []


def test_check_drift_reports_a_missing_block():
    problems = check_drift(AGENTS.format(lane=PR_AND_MERGE))
    assert len(problems) == 1
    assert "missing the managed git-workflow block" in problems[0]


def test_check_drift_reports_a_hand_edit_inside_the_block():
    out = apply_to_text(AGENTS.format(lane=PR_AND_MERGE))
    edited = out.replace("**ALWAYS commit**", "maybe commit", 1)
    problems = check_drift(edited)
    assert len(problems) == 1
    assert "drifted from generator output" in problems[0]


def test_check_drift_reports_a_legacy_stamp_surviving_beside_the_block():
    out = apply_to_text(AGENTS.format(lane=PR_AND_MERGE))
    half_migrated = out.replace(
        "## Agent rules\n", "## Agent rules\n\n**Git workflow** - `pull-request`.\n", 1
    )
    problems = check_drift(half_migrated)
    assert len(problems) == 1
    assert "legacy one-line git-workflow stamp survives" in problems[0]


def test_this_repo_carries_a_current_block():
    from pathlib import Path

    root = Path(__file__).resolve().parent.parent
    assert check_drift((root / "AGENTS.md").read_text(encoding="utf-8")) == []
