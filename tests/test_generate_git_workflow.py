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
    main,
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
    assert detect_lane(AGENTS.format(lane=PULL_REQUEST)) == PULL_REQUEST
    assert detect_lane(AGENTS.format(lane=PR_AND_MERGE)) == PR_AND_MERGE


def test_detect_lane_refuses_the_retired_slug():
    # Retired at normalize, which detect_lane runs through, so a repo still
    # declaring it reads as undeclared and renders the guarded shape.
    assert detect_lane(AGENTS.format(lane=MERGE_MAIN)) is None


def test_detect_lane_is_none_without_a_declaration():
    assert detect_lane(NO_FRONTMATTER) is None
    assert detect_lane("---\nward: not-a-mapping\n---\n# x\n") is None
    assert detect_lane("---\nward:\n  workflow: invented-lane\n---\n# x\n") is None


def test_normalize_lane_rejects_non_strings_and_unknown_slugs():
    assert normalize_lane(None) is None
    assert normalize_lane(42) is None
    assert normalize_lane("push-whatever") is None


def test_the_retired_merge_remote_main_slug_no_longer_normalizes():
    # Padded too: normalize strips before matching, and a retired slug must
    # not come back through that door.
    assert normalize_lane(MERGE_MAIN) is None
    assert normalize_lane("  merge-remote-main  ") is None
    assert MERGE_MAIN not in LANES


@pytest.mark.parametrize("lane", LANES)
def test_every_lane_renders_a_marker_delimited_block(lane):
    block = render_block(lane)
    assert block.startswith(BEGIN)
    assert block.endswith(END)
    assert f"**This repo runs the `{lane}` lane**" in block


def test_undeclared_lane_grants_neither_a_main_push_nor_a_merge():
    body = render_body(None)
    assert "declares no `ward.workflow` lane" in body
    assert f"MUST work the `{PULL_REQUEST}` shape" in body
    assert "No direct push to `main`, and no agent merge." in body
    # An unknown slug is undeclared, not a lane of its own.
    assert render_body("invented-lane") == body


def test_block_names_the_one_fleet_lane_whichever_one_is_active():
    for lane in (PULL_REQUEST, PR_AND_MERGE, None):
        block = render_block(lane)
        assert f"* `{PR_AND_MERGE}` -" in block
        # The retired slug is named as retired, never offered as a bullet.
        assert f"* `{MERGE_MAIN}` -" not in block
        assert f"`{MERGE_MAIN}` is retired" in block


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
    out = apply_to_text(AGENTS.format(lane=PULL_REQUEST))
    assert f"**This repo runs the `{PULL_REQUEST}` lane**" in out
    assert check_drift(out) == []


def test_apply_gives_a_repo_on_the_retired_lane_the_guarded_shape():
    # Never a direct push on a slug the generator no longer honors.
    out = apply_to_text(AGENTS.format(lane=MERGE_MAIN))
    assert "**This repo declares no `ward.workflow` lane.**" in out
    assert check_drift(out) == []


def test_apply_is_idempotent():
    once = apply_to_text(AGENTS.format(lane=PR_AND_MERGE))
    assert apply_to_text(once) == once
    assert once.count(BEGIN) == 1


def test_apply_refreshes_a_block_left_on_a_stale_lane():
    stale = apply_to_text(AGENTS.format(lane=PULL_REQUEST))
    relaned = stale.replace(f"workflow: {PULL_REQUEST}", f"workflow: {PR_AND_MERGE}", 1)
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


# The slug names AGENT behavior, and two drafts inverted the PR lanes.
# These pin the direction rather than the wording.


def test_and_merge_lane_makes_the_author_merge_its_own_pull_request():
    block = render_block(PR_AND_MERGE)
    assert "**merges that pull request itself**" in block
    assert "The author of the code is the one who merges it." in block
    assert "Opening the pull request is a step, never the stopping point." in block


def test_plain_pull_request_lane_stops_at_the_pull_request():
    block = render_block(PULL_REQUEST)
    assert "The author does not merge on this lane." in block
    assert "director merge lane takes it from the pull request onward" in block


def test_block_states_the_slug_names_agent_behavior():
    block = render_block(PR_AND_MERGE)
    assert "names what the AGENT does, never what someone else does" in block
    assert f'Reading `{PR_AND_MERGE}` as "someone else merges it later" inverts' in block


def test_every_lane_carries_both_merge_directions():
    for lane in LANES:
        block = render_block(lane)
        assert f"**ALWAYS merge your own pull request on `{PR_AND_MERGE}`**" in block
        assert f"**NEVER merge on `{PULL_REQUEST}` or `{BRANCH_ONLY}`.**" in block


def test_no_lane_hands_the_and_merge_pull_request_to_someone_else():
    for lane in LANES:
        block = render_block(lane)
        for inversion in ("director-gated", "The human merges", "human-gated"):
            assert inversion not in block


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


def _print_lane(tmp_path, capsys, body: str | None) -> str:
    """Run `--print-lane` against an AGENTS.md, or against none at all."""
    agents = tmp_path / "AGENTS.md"
    if body is not None:
        agents.write_text(body, encoding="utf-8")
    assert main(["--print-lane", "--agents-md", str(agents)]) == 0
    return capsys.readouterr().out


@pytest.mark.parametrize("lane", LANES)
def test_print_lane_echoes_every_declared_lane(tmp_path, capsys, lane):
    assert _print_lane(tmp_path, capsys, AGENTS.format(lane=lane)) == f"{lane}\n"


def test_print_lane_prints_nothing_for_an_undeclared_repo(tmp_path, capsys):
    # pr-guard reads an empty answer as undeclared and keeps guarding, so a
    # fallback slug here would hand it an authority the repo never granted.
    assert _print_lane(tmp_path, capsys, AGENTS.format(lane="not-a-lane")) == ""


def test_print_lane_prints_nothing_and_warns_nothing_without_an_agents_md(
    tmp_path, capsys
):
    agents = tmp_path / "AGENTS.md"
    assert main(["--print-lane", "--agents-md", str(agents)]) == 0
    captured = capsys.readouterr()
    assert captured.out == ""
    assert captured.err == ""


def test_print_lane_never_prints_the_block(tmp_path, capsys):
    out = _print_lane(tmp_path, capsys, AGENTS.format(lane=MERGE_MAIN))
    assert BEGIN not in out and END not in out
