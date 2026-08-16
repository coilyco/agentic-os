#!/usr/bin/env python3
"""Seed-skill definitions: which skills propagate into target repos.

qwen-opencode's per-repo context management wants a small amount of language
context living inside each target repo. A composed coding source opts into
that propagation with a ``seed:`` block in its COMPOSED.md frontmatter:

    seed:
      kind: always                    # seed into every target repo (baseline)

    seed:
      kind: language
      language: python
      extensions: [".py", ".pyi"]     # seed into repos containing these files

agentic-os is the source of truth. The role-gated ``coding-<lang>`` sources
carry the frontmatter. The ``seed-skills`` hook runs in consumer repos without
this source tree, so ``generate-seed-skills`` writes the table into
``seed_skills_data.py`` for the package. ``check-seed-skills-drift`` fails when
that generated file is stale and is dogfooded in agentic-os only.

Downstream repos reference a seeded skill by its canonical path,
``.agents/skills/coding-python/SKILL.md`` (as a relative ref or under the full
``forgejo.coilysiren.me/coilyco-flight-deck/agentic-os`` URL). The presence
check also accepts the canonical composed source path. ``canonical_ref`` builds
the delivered path tail, ``source_ref`` builds the source path tail, and
``suggested_url`` builds the full Forgejo URL shown in guidance.

Schema and rollout: see docs/skill-discipline-authoring.md.
"""
from __future__ import annotations

import difflib
import sys
from pathlib import Path
from typing import NoReturn

from agentic_os.pre_commit.check_skill import parse_frontmatter

TRACKER = "docs/skill-discipline-authoring.md"

_REPO_ROOT = Path(__file__).resolve().parents[2]
COMPOSED_SKILLS_DIR = _REPO_ROOT / ".agents" / "composed"
DATA_PATH = Path(__file__).resolve().parents[1] / "seed_skills_data.py"

DELIVERED_SKILLS_PREFIX = ".agents/skills"
SOURCE_SKILLS_PREFIX = ".agents/composed"
CANONICAL_REPO = "forgejo.coilysiren.me/coilyco-flight-deck/agentic-os"


def canonical_ref(skill: str) -> str:
    """The path tail a target repo must reference for this skill to count."""
    return f"{DELIVERED_SKILLS_PREFIX}/{skill}/SKILL.md"


def source_ref(skill: str) -> str:
    """The canonical role-composed source path for a seed skill."""
    return f"{SOURCE_SKILLS_PREFIX}/{skill}/COMPOSED.md"


def suggested_url(skill: str) -> str:
    """Full Forgejo URL shown in guidance when a reference is missing."""
    return f"https://{CANONICAL_REPO}/src/branch/main/{source_ref(skill)}"


def _parse_seed_block(name: str, seed: object) -> tuple[str, dict | None]:
    """Validate a frontmatter ``seed:`` block, return (always|language, lang).

    Returns ``("always", None)`` for the baseline kind and
    ``("language", {"language": ..., "extensions": [...]})`` for a language
    kind. Raises ``ValueError`` on a malformed block.
    """
    if not isinstance(seed, dict):
        raise ValueError(f"{name}: seed: must be a mapping")
    kind = seed.get("kind")
    if kind == "always":
        return "always", None
    if kind == "language":
        language = seed.get("language")
        extensions = seed.get("extensions")
        if not isinstance(language, str) or not language.strip():
            raise ValueError(f"{name}: seed.language must be a non-empty string")
        if not isinstance(extensions, list) or not extensions or not all(
            isinstance(e, str) and e.startswith(".") for e in extensions
        ):
            raise ValueError(
                f"{name}: seed.extensions must be a non-empty list of "
                f'".ext" strings'
            )
        return "language", {"language": language, "extensions": list(extensions)}
    raise ValueError(f"{name}: seed.kind must be 'always' or 'language', got {kind!r}")


def iter_seed_skills(
    composed_dir: Path = COMPOSED_SKILLS_DIR,
) -> tuple[list[str], dict[str, dict]]:
    """Scan composed coding frontmatter, return (always_skills, languages).

    ``always_skills`` is a sorted list of skill folder names with
    ``seed.kind: always``. ``languages`` maps language id -> {"skill", "extensions"}.
    Raises ``ValueError`` on a malformed seed block or a duplicate language /
    extension claim.
    """
    always: list[str] = []
    languages: dict[str, dict] = {}
    ext_owner: dict[str, str] = {}
    for skill_dir in sorted(p for p in composed_dir.iterdir() if p.is_dir()):
        md = skill_dir / "COMPOSED.md"
        if not md.is_file():
            continue
        fm, _ = parse_frontmatter(md.read_text(encoding="utf-8"))
        if not fm or "seed" not in fm:
            continue
        name = skill_dir.name
        kind, lang = _parse_seed_block(name, fm["seed"])
        if kind == "always":
            always.append(name)
            continue
        assert lang is not None
        lang_id = lang["language"]
        if lang_id in languages:
            raise ValueError(
                f"language {lang_id!r} claimed by both {languages[lang_id]['skill']} "
                f"and {name}"
            )
        for ext in lang["extensions"]:
            if ext in ext_owner:
                raise ValueError(
                    f"extension {ext!r} claimed by both {ext_owner[ext]} and {name}"
                )
            ext_owner[ext] = name
        languages[lang_id] = {"skill": name, "extensions": lang["extensions"]}
    return sorted(always), dict(sorted(languages.items()))


def render_data_module(always: list[str], languages: dict[str, dict]) -> str:
    """Deterministic source for seed_skills_data.py. No timestamps."""
    lines = [
        '"""Generated by `generate-seed-skills` from composed skill frontmatter.',
        "",
        "Do not edit by hand. Run `generate-seed-skills` after changing a",
        "skill's seed: frontmatter; check-seed-skills-drift fails on staleness.",
        '"""',
        "from __future__ import annotations",
        "",
        f"SEED_ALWAYS: list[str] = {always!r}",
        "",
        "SEED_LANGUAGES: dict[str, dict] = {",
    ]
    for lang_id, info in languages.items():
        lines.append(
            f"    {lang_id!r}: {{"
            f'"skill": {info["skill"]!r}, '
            f'"extensions": {info["extensions"]!r}}},'
        )
    lines.append("}")
    return "\n".join(lines) + "\n"


def load_data() -> tuple[list[str], dict[str, dict]]:
    """Read the generated table shipped in the package (consumer-side).

    Returns ([], {}) if the data module is absent (pre-generation), so callers
    degrade to a no-op rather than crashing.
    """
    try:
        from agentic_os import seed_skills_data as data
    except ImportError:
        return [], {}
    return list(data.SEED_ALWAYS), dict(data.SEED_LANGUAGES)


def generate(
    composed_dir: Path = COMPOSED_SKILLS_DIR, data_path: Path = DATA_PATH
) -> int:
    always, languages = iter_seed_skills(composed_dir)
    data_path.write_text(render_data_module(always, languages), encoding="utf-8")
    print(
        f"generate-seed-skills: wrote {data_path.name} "
        f"({len(always)} always, {len(languages)} language(s))"
    )
    return 0


def check_drift(
    composed_dir: Path = COMPOSED_SKILLS_DIR, data_path: Path = DATA_PATH
) -> int:
    always, languages = iter_seed_skills(composed_dir)
    expected = render_data_module(always, languages)
    actual = data_path.read_text(encoding="utf-8") if data_path.exists() else ""
    if actual == expected:
        print(f"seed-skills-drift: {data_path.name} in sync")
        return 0
    sys.stderr.write(
        f"seed-skills-drift: {data_path.name} is missing, stale, or hand-edited. "
        "Run `generate-seed-skills` to regenerate.\n"
    )
    diff = difflib.unified_diff(
        actual.splitlines(),
        expected.splitlines(),
        fromfile=f"{data_path.name} (on disk)",
        tofile="expected (fresh generate)",
        lineterm="",
    )
    for line in list(diff)[:40]:
        sys.stderr.write(line + "\n")
    return 1


def main() -> int:
    """Console entry for generate-seed-skills. `--check` reports drift instead."""
    import argparse

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check",
        action="store_true",
        help="verify seed_skills_data.py is in sync; exit 1 on drift",
    )
    args = parser.parse_args()
    try:
        return check_drift() if args.check else generate()
    except ValueError as exc:
        _fail(str(exc))


def check_drift_main() -> int:
    """Console entry for the check-seed-skills-drift pre-commit hook."""
    try:
        return check_drift()
    except ValueError as exc:
        _fail(str(exc))


def _fail(msg: str) -> NoReturn:
    sys.stderr.write(f"seed-skills: malformed seed frontmatter - {msg}\n")
    sys.stderr.write(f"  see {TRACKER}\n")
    sys.exit(1)


if __name__ == "__main__":
    sys.exit(main())
