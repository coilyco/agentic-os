import re

from agentic_os.generators.generate_repo_pointer_skill import (
    DEFAULT_ORG,
    build_description,
    check_drift,
    clean_description,
    parse_frontmatter,
    repo_skill_name,
    render_skill,
)


def test_clean_description_strips_emoji_and_normalizes_dashes():
    out = clean_description("🚀 coilysiren.me — personal site 🧠")
    assert "🚀" not in out and "🧠" not in out
    assert "—" not in out
    assert out == "coilysiren.me - personal site"


def test_build_description_prepends_name_and_dedups_topics():
    desc = build_description("Personal site.", "website", ["website", "personal-site"])
    assert desc == "Personal site. Triggers - website, personal-site"


def test_build_description_handles_empty_topics():
    assert build_description("A backend.", "backend", []) == "A backend. Triggers - backend"


def test_render_round_trips_through_frontmatter_parse():
    desc = build_description("coilysiren.me — site 🚀", "website", ["personal-site"])
    text = render_skill("website", desc)
    fm = parse_frontmatter(text)
    assert fm["name"] == "repo-website"
    assert fm["description"] == desc
    # Re-rendering from the parsed description is idempotent.
    assert render_skill("website", fm["description"]) == text


def test_check_drift_passes_clean_generated_file():
    desc = build_description("coilysiren.me — site 🚀", "website", ["personal-site"])
    text = render_skill("website", desc)
    assert check_drift("repo-website", text) == []


def test_render_skill_default_org_is_the_fleet_home_org():
    """The default is the guess for a repo that sets no org. Every repo with the
    hook enabled overrides it, so this locks the fallback rather than a live path."""
    text = render_skill("newrepo", "A new repo. Triggers - newrepo")
    assert "Repository `coilyco-bridge/newrepo`." in text
    assert "`~/projects/coilyco-bridge/newrepo/` when resident." in text
    assert check_drift("repo-newrepo", text) == []


def test_render_skill_org_overrides_pointer_path():
    text = render_skill("deploy", "A monorepo. Triggers - deploy", "coilyco-bridge")
    assert "Repository `coilyco-bridge/deploy`. Checkout at `~/projects/coilyco-bridge/deploy/` when resident." in text
    assert "coilysiren" not in text


def test_repo_skill_name_preserves_ordinary_repository_names():
    assert repo_skill_name("website", "coilysiren") == "repo-website"


def test_repo_skill_name_distinguishes_dot_repositories_by_owner():
    bridge = repo_skill_name(".github", "coilyco-bridge")
    flight_deck = repo_skill_name(".github", "coilyco-flight-deck")

    assert bridge.startswith("repo-coilyco-bridge-github-")
    assert flight_deck.startswith("repo-coilyco-flight-deck-github-")
    assert bridge != flight_deck
    assert re.fullmatch(r"repo-[a-z0-9][a-z0-9-]*", bridge)
    assert re.fullmatch(r"repo-[a-z0-9][a-z0-9-]*", flight_deck)


def test_dot_repository_render_and_drift_check_preserve_real_path():
    text = render_skill(
        ".github",
        "Organization profile. Triggers - .github",
        "coilyco-bridge",
    )
    skill_name = repo_skill_name(".github", "coilyco-bridge")

    assert f"name: {skill_name}" in text
    assert "Repository `coilyco-bridge/.github`. Checkout at `~/projects/coilyco-bridge/.github/` when resident." in text
    assert check_drift(skill_name, text, "coilyco-bridge") == []


def test_check_drift_uses_org_for_byte_match():
    # Derived, not named: the second assertion goes vacuous if this equals
    # DEFAULT_ORG, which is how the hardcoded version broke.
    other = "coilyco-flight-deck" if DEFAULT_ORG != "coilyco-flight-deck" else "coilyco-gaming"
    desc = "A monorepo. Triggers - deploy"
    migrated = render_skill("deploy", desc, other)
    # Clean under the matching org, drifted when checked as the default org.
    assert check_drift("repo-deploy", migrated, other) == []
    assert any("drifted" in p for p in check_drift("repo-deploy", migrated))


def test_check_drift_flags_hand_edited_body():
    text = render_skill("website", "A site. Triggers - website")
    tampered = text.replace("what ships today", "WHAT SHIPS TODAY")
    problems = check_drift("repo-website", tampered)
    assert any("drifted" in p for p in problems)


def test_check_drift_flags_emoji_in_description():
    text = render_skill("website", "A site 🚀. Triggers - website")
    problems = check_drift("repo-website", text)
    assert any("emoji" in p for p in problems)


def test_check_drift_flags_em_dash_in_description():
    text = render_skill("website", "A site — really. Triggers - website")
    problems = check_drift("repo-website", text)
    assert any("dash" in p for p in problems)


def test_check_drift_flags_missing_triggers_line():
    text = render_skill("website", "A site with no trigger line")
    problems = check_drift("repo-website", text)
    assert any("Triggers" in p for p in problems)


def test_check_drift_flags_name_mismatch():
    text = render_skill("website", "A site. Triggers - website")
    problems = check_drift("repo-other", text)
    assert any("does not match" in p for p in problems)


def test_check_drift_rejects_non_prefixed_dir():
    text = render_skill("website", "A site. Triggers - website")
    problems = check_drift("website", text)
    assert problems and "prefix" in problems[0]
