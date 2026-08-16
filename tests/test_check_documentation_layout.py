"""Tests for agentic_os.pre_commit.check_documentation_layout skill flatness rule.

The flatness rule targets nested sub-skills (a SKILL.md the loader can't see),
not support material that legitimately sits beside SKILL.md.
"""
from __future__ import annotations

from pathlib import Path

import agentic_os.config as config
import agentic_os.pre_commit.check_documentation_layout as docs_layout
from agentic_os.pre_commit.check_documentation_layout import (
    AGENTS_DEFAULT_MAX_CHARS,
    AGENTS_DEFAULT_MAX_LINES,
    ROOT_MARKDOWN_ALLOWLIST,
    TRIFECTA_MAX_CHARS,
    TRIFECTA_MAX_LINES,
    caps_for,
    check_skill_flatness,
    is_harness_override,
    validate_module_readme,
)


def write(path: Path, text: str = "x\n") -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


def _point_repo_root_at(tmp_path: Path, monkeypatch) -> None:
    # Reaches both REPO_ROOT (the tree walk) and config.REPO_ROOT (the options
    # and excludes a fixture repo declares in its own pyproject.toml).
    monkeypatch.setattr(docs_layout, "REPO_ROOT", tmp_path)
    monkeypatch.setattr(config, "REPO_ROOT", tmp_path)


def test_features_rides_the_band_like_any_docs_page(tmp_path: Path, monkeypatch) -> None:
    # FEATURES.md carried its own constant that happened to equal the ordinary
    # cap, so it was a special case in name only. It now takes the band.
    _point_repo_root_at(tmp_path, monkeypatch)
    assert caps_for(Path("docs/FEATURES.md")) == docs_layout.markdown_caps()


def test_overview_caps_fall_back_to_the_shared_defaults(
    tmp_path: Path, monkeypatch
) -> None:
    # A repo that declares no keys rides the shared defaults: README.md on the
    # overview cap, AGENTS.md on its larger one.
    _point_repo_root_at(tmp_path, monkeypatch)
    assert caps_for(Path("README.md")) == (TRIFECTA_MAX_LINES, TRIFECTA_MAX_CHARS)
    assert caps_for(Path("AGENTS.md")) == (
        AGENTS_DEFAULT_MAX_LINES,
        AGENTS_DEFAULT_MAX_CHARS,
    )
    assert AGENTS_DEFAULT_MAX_LINES > TRIFECTA_MAX_LINES
    assert AGENTS_DEFAULT_MAX_CHARS > TRIFECTA_MAX_CHARS


def test_overview_caps_take_the_declared_value_in_either_direction(
    tmp_path: Path, monkeypatch
) -> None:
    # The keys replace the default outright: one repo may buy README headroom
    # and tighten AGENTS.md as back-pressure in the same declaration.
    write(
        tmp_path / "pyproject.toml",
        "[tool.agentic-os.documentation-layout]\n"
        "readme_max_lines = 400\n"
        "readme_max_chars = 30000\n"
        "agents_md_max_lines = 40\n"
        "agents_md_max_chars = 3000\n",
    )
    _point_repo_root_at(tmp_path, monkeypatch)
    assert caps_for(Path("README.md")) == (400, 30_000)
    assert caps_for(Path("AGENTS.md")) == (40, 3_000)


def test_non_trifecta_markdown_keeps_the_standard_cap() -> None:
    # Only the root README breathes; a co-located module README and ordinary
    # docs/*.md stay on the tight cap.
    assert caps_for(Path("docs/o11y.md")) == docs_layout.markdown_caps()
    assert caps_for(Path("services/x/README.md")) == docs_layout.markdown_caps()


def test_code_review_md_keeps_the_standard_cap() -> None:
    assert caps_for(Path("CODE-REVIEW.md")) == docs_layout.markdown_caps()


def test_agents_compose_md_is_an_allowed_root_file() -> None:
    # agent-compose's disjoint source is a repo-root convention; the layout
    # rule must not reject it the way it rejects one-off root Markdown.
    assert "AGENTS.COMPOSE.md" in ROOT_MARKDOWN_ALLOWLIST


def test_code_review_md_is_an_allowed_root_file() -> None:
    # CODE-REVIEW.md is a root contract doc, not a docs/ file.
    assert "CODE-REVIEW.md" in ROOT_MARKDOWN_ALLOWLIST


def test_harness_override_filenames_are_recognized() -> None:
    # AGENTS.<harness>.md overrides sit at repo root beside AGENTS.md.
    assert is_harness_override("AGENTS.codex.md")
    assert is_harness_override("AGENTS.claude.md")
    # not overrides: the uppercase disjoint source, the base, one-off docs.
    assert not is_harness_override("AGENTS.COMPOSE.md")
    assert not is_harness_override("AGENTS.md")
    assert not is_harness_override("notes.md")


def test_support_subdirs_are_allowed(tmp_path: Path) -> None:
    skill = tmp_path / ".agents" / "skills" / "my-skill"
    write(skill / "SKILL.md")
    write(skill / "scripts" / "run.sh")
    write(skill / "assets" / "logo.png")
    write(skill / "agents" / "openai.yaml")
    write(skill / "references" / "deep.md")
    assert check_skill_flatness(tmp_path) == []


def test_nested_skill_md_is_flagged(tmp_path: Path) -> None:
    skill = tmp_path / ".agents" / "skills" / "my-skill"
    write(skill / "SKILL.md")
    write(skill / "sub-skill" / "SKILL.md")
    problems = check_skill_flatness(tmp_path)
    assert len(problems) == 1
    assert "sub-skill/SKILL.md" in problems[0]
    assert "nested SKILL.md" in problems[0]


def test_top_level_skill_md_is_clean(tmp_path: Path) -> None:
    for name in ("a", "b", "c"):
        write(tmp_path / ".agents" / "skills" / name / "SKILL.md")
    assert check_skill_flatness(tmp_path) == []


def test_top_level_composed_md_is_clean(tmp_path: Path) -> None:
    write(tmp_path / ".agents" / "composed" / "my-skill" / "COMPOSED.md")
    assert check_skill_flatness(tmp_path) == []


def test_nested_composed_md_is_flagged(tmp_path: Path) -> None:
    skill = tmp_path / ".agents" / "composed" / "my-skill"
    write(skill / "COMPOSED.md")
    write(skill / "sub-skill" / "COMPOSED.md")
    problems = check_skill_flatness(tmp_path)
    assert len(problems) == 1
    assert "nested COMPOSED.md" in problems[0]


def test_nested_skill_md_can_be_excluded(tmp_path: Path) -> None:
    skill = tmp_path / ".agents" / "skills" / "my-skill"
    write(skill / "SKILL.md")
    write(skill / "vendor" / "SKILL.md")
    write(
        tmp_path / "pyproject.toml",
        '[tool.agentic-os.documentation-layout]\n'
        'excludes = [".agents/skills/my-skill/vendor/**"]\n',
    )
    assert check_skill_flatness(tmp_path) == []


# Module README.md: outpost / homestead shapes. validate_module_readme takes a
# repo-relative README path and the repo root, returning [] when valid.

def readme(text: str) -> str:
    return text


def test_valid_outpost_with_reciprocal_docs(tmp_path: Path) -> None:
    write(
        tmp_path / "ansible" / "README.md",
        "# Ansible\nConverges workstation state.\n"
        "Full runbook: [docs/ansible.md](../docs/ansible.md)\n",
    )
    write(tmp_path / "docs" / "ansible.md", "# Ansible\nSee [ansible/](../ansible/README.md).\n")
    assert validate_module_readme(Path("ansible/README.md"), tmp_path) == []


def test_outpost_without_back_link_is_flagged(tmp_path: Path) -> None:
    write(
        tmp_path / "ansible" / "README.md",
        "# Ansible\n[docs/ansible.md](../docs/ansible.md)\n",
    )
    write(tmp_path / "docs" / "ansible.md", "# Ansible\nNo link back here.\n")
    problems = validate_module_readme(Path("ansible/README.md"), tmp_path)
    assert len(problems) == 1
    assert "not reciprocal" in problems[0]


def test_outpost_pointing_at_missing_doc_is_flagged(tmp_path: Path) -> None:
    write(
        tmp_path / "ansible" / "README.md",
        "# Ansible\n[gone](../docs/ansible.md)\n",
    )
    problems = validate_module_readme(Path("ansible/README.md"), tmp_path)
    assert len(problems) == 1
    assert "does not exist" in problems[0]


def test_outpost_with_two_docs_targets_is_flagged(tmp_path: Path) -> None:
    write(
        tmp_path / "ansible" / "README.md",
        "# Ansible\n[a](../docs/a.md) and [b](../docs/b.md)\n",
    )
    write(tmp_path / "docs" / "a.md", "[x](../ansible/README.md)\n")
    write(tmp_path / "docs" / "b.md", "[x](../ansible/README.md)\n")
    problems = validate_module_readme(Path("ansible/README.md"), tmp_path)
    assert any("exactly one" in p for p in problems)


def test_outpost_pointer_line_exempt_from_char_cap(tmp_path: Path) -> None:
    # A long relative path on the pointer line must not trip the prose cap.
    deep = "deploy/some/very/deeply/nested/module"
    target = "../" * 6 + "docs/deploy-some-very-deeply-nested-module.md"
    write(
        tmp_path / deep / "README.md",
        f"# Module\n[full runbook with a long path]({target})\n",
    )
    write(
        tmp_path / "docs" / "deploy-some-very-deeply-nested-module.md",
        f"# Module\n[x](/{deep}/README.md)\n",
    )
    assert validate_module_readme(Path(f"{deep}/README.md"), tmp_path) == []


def test_valid_homestead(tmp_path: Path) -> None:
    write(
        tmp_path / "eco-server" / "README.md",
        "# Eco server\nVendored game-server tree.\nDo not edit by hand.\n",
    )
    assert validate_module_readme(Path("eco-server/README.md"), tmp_path) == []


def test_homestead_over_line_cap_is_flagged(tmp_path: Path) -> None:
    write(
        tmp_path / "mod" / "README.md",
        "# Mod\nline one\nline two\nline three\n",
    )
    problems = validate_module_readme(Path("mod/README.md"), tmp_path)
    assert any("non-blank lines" in p for p in problems)


def test_blank_lines_do_not_count_toward_line_cap(tmp_path: Path) -> None:
    write(
        tmp_path / "mod" / "README.md",
        "# Mod\n\nVendored tree.\n\nDo not edit.\n",
    )
    assert validate_module_readme(Path("mod/README.md"), tmp_path) == []


def test_homestead_prose_over_char_cap_is_flagged(tmp_path: Path) -> None:
    write(
        tmp_path / "mod" / "README.md",
        "# Mod\n" + "x" * 91 + "\n",
    )
    problems = validate_module_readme(Path("mod/README.md"), tmp_path)
    assert any("chars, max 90" in p for p in problems)


def test_readme_must_lead_with_heading(tmp_path: Path) -> None:
    write(tmp_path / "mod" / "README.md", "just text, no heading\n")
    problems = validate_module_readme(Path("mod/README.md"), tmp_path)
    assert any("heading" in p for p in problems)


def test_root_absolute_back_link_is_reciprocal(tmp_path: Path) -> None:
    # docs file may link back root-absolute (/ansible/README.md), not relative.
    write(
        tmp_path / "ansible" / "README.md",
        "# Ansible\n[runbook](/docs/ansible.md)\n",
    )
    write(tmp_path / "docs" / "ansible.md", "# Ansible\n[home](/ansible/README.md)\n")
    assert validate_module_readme(Path("ansible/README.md"), tmp_path) == []


# Generated-document exclusion checks that a wildcard reaches both REPO_ROOT
# (the tree walk) and config.REPO_ROOT (the excludes).

def _write_generated_docs(tmp_path: Path) -> None:
    # Oversized (> 80-line cap) generated docs emitted at two locations.
    big = "# Generated reference\n" + "\n".join(f"verb {i}" for i in range(120))
    for gen in ("aws", "open-webui", "forgejo"):
        write(tmp_path / "docs" / f"generated.{gen}.md", big)
        write(tmp_path / "cmd" / "generated" / f"generated.{gen}.md", big)


def test_wildcard_exclude_clears_placement_but_never_size(
    tmp_path: Path, monkeypatch
) -> None:
    _write_generated_docs(tmp_path)
    write(
        tmp_path / "pyproject.toml",
        "[tool.agentic-os.documentation-layout]\n"
        'excludes = ["generated.*.md"]\n',
    )
    _point_repo_root_at(tmp_path, monkeypatch)
    # A wildcard still silences the location rule for the cmd/ copies, because
    # where a generated file lands is a layout decision.
    assert docs_layout.check_markdown_locations() == []
    # It never reaches the size cap: an oversized generated doc is a generator
    # emitting too much, and the generator is the fix.
    assert docs_layout.check_markdown_sizes() != []


def test_generated_guardfiles_flagged_without_exclude(tmp_path: Path, monkeypatch) -> None:
    _write_generated_docs(tmp_path)
    _point_repo_root_at(tmp_path, monkeypatch)
    # Sanity check the exclude is doing the work: the cmd/ copies are mislocated
    # and every copy is oversized when nothing is excluded.
    locations = docs_layout.check_markdown_locations()
    sizes = docs_layout.check_markdown_sizes()
    assert any("cmd/generated/generated.aws.md" in v for v in locations)
    assert len(sizes) >= len(("aws", "open-webui", "forgejo")) * 2


# Size bands: a repo declares small or large and gets that band's three caps.

def test_no_declaration_is_a_violation(tmp_path: Path, monkeypatch) -> None:
    # Small is not the silent default. An undeclared repo is a repo whose band
    # nobody decided, which reads identically to a deliberate small.
    _point_repo_root_at(tmp_path, monkeypatch)
    assert docs_layout.check_band_declaration() != []
    # Caps still resolve to the tight band while the repo is red, so the size
    # checks stay meaningful rather than crashing or passing everything.
    assert docs_layout.band() == "small"
    assert docs_layout.markdown_caps() == (40, 3_000)
    assert docs_layout.docs_cap() == 20


def test_declared_small_band_is_accepted(tmp_path: Path, monkeypatch) -> None:
    write(
        tmp_path / "pyproject.toml",
        '[tool.agentic-os.documentation-layout]\nband = "small"\n',
    )
    _point_repo_root_at(tmp_path, monkeypatch)
    assert docs_layout.check_band_declaration() == []
    assert docs_layout.markdown_caps() == (40, 3_000)
    assert docs_layout.docs_cap() == 20


def test_declared_large_band_raises_all_three_caps(tmp_path: Path, monkeypatch) -> None:
    write(
        tmp_path / "pyproject.toml",
        '[tool.agentic-os.documentation-layout]\nband = "large"\n',
    )
    _point_repo_root_at(tmp_path, monkeypatch)
    assert docs_layout.markdown_caps() == (120, 8_000)
    assert docs_layout.docs_cap() == 40


def test_unknown_band_is_a_violation_not_a_silent_default(
    tmp_path: Path, monkeypatch
) -> None:
    # A typo must fail loudly. Falling back to small silently would let a repo
    # believe it declared large while being measured against small.
    write(
        tmp_path / "pyproject.toml",
        '[tool.agentic-os.documentation-layout]\nband = "medium"\n',
    )
    _point_repo_root_at(tmp_path, monkeypatch)
    assert docs_layout.check_band_declaration() != []


def test_docs_count_cap_fires_past_the_band(tmp_path: Path, monkeypatch) -> None:
    for i in range(21):
        write(tmp_path / "docs" / f"page-{i}.md", "# Page\n")
    _point_repo_root_at(tmp_path, monkeypatch)
    assert docs_layout.check_docs_count() != []
    write(
        tmp_path / "pyproject.toml",
        '[tool.agentic-os.documentation-layout]\nband = "large"\n',
    )
    assert docs_layout.check_docs_count() == []


def test_excludes_cannot_shrink_the_docs_count(tmp_path: Path, monkeypatch) -> None:
    # An exclude that hid pages from the count would make the cap a formality.
    for i in range(21):
        write(tmp_path / "docs" / f"page-{i}.md", "# Page\n")
    write(
        tmp_path / "pyproject.toml",
        "[tool.agentic-os.documentation-layout]\n"
        'excludes = ["docs/page-*.md"]\n',
    )
    _point_repo_root_at(tmp_path, monkeypatch)
    assert docs_layout.check_docs_count() != []
