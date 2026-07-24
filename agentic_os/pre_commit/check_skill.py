#!/usr/bin/env python3
"""
Validator for a repo's skill directory surface.

Enforces structural rules driven by the skill directory's categories.yaml.
The consumer-facing handbook is shipped alongside this script in
agentic-os/docs/skill-discipline-handbook.md.

Usage (when run directly):
    check-skills              # validate every skill
    check-skills <name> ...   # validate only the named skills
    check-skills --report-only  # exit 0 even on failures

Canonical copy lives in coilyco-flight-deck/agentic-os/scripts/. Each consumer
repo gets a stamped copy via agentic-os-kai's apply-skill-discipline-hooks
rollout. Exits 0 on success, 1 on any failure with a per-violation report
on stderr.
"""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path

try:
    import yaml
except ModuleNotFoundError:
    sys.stderr.write(
        "check-skills: PyYAML is required. Install with: pip install pyyaml\n"
    )
    sys.exit(2)

from agentic_os.config import is_enabled

HOOK_ID = "check-skills"

# cwd is the consumer repo root under pre-commit, same as direct dev runs.
REPO_ROOT = Path.cwd()
SKILLS_DIR = REPO_ROOT / ".claude" / "skills"
SPEC_PATH = SKILLS_DIR / "categories.yaml"
ENTRYPOINT_NAME = "SKILL.md"

# Skill directory layout, canonical first. .agents/skills is the current home.
SKILLS_DIR_CANDIDATES = (".agents/skills", ".claude/skills", "skills")


def detect_skills_dir() -> str:
    for candidate in SKILLS_DIR_CANDIDATES:
        if (REPO_ROOT / candidate).is_dir():
            return candidate
    return SKILLS_DIR_CANDIDATES[0]

DEFAULT_MAX_LINES = 500
DEFAULT_MAX_BYTES = 10_000
DEFAULT_MAX_DESCRIPTION_BYTES = 500

# Thin skills (role: thin) must fit a small-local-model catalog budget. Cap is
# 1/4 the 4000-char / 80-line cap. See docs/skill-discipline-handbook-size-caps.md.
THIN_MAX_LINES = 20
THIN_MAX_BYTES = 1_000

STATUS_LINE_RE = re.compile(
    r"^Status:\s+(?P<emoji>\S+)\s+(?P<kind>[A-Za-z]+)\s+\|\s+Last\s+(?P<freshness>updated|tested):\s+(?P<date>\d{4}-\d{2}-\d{2})\s*$"
)

MD_LINK_RE = re.compile(r"\[(?P<text>[^\]\n]+)\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)")

# Backticked references like `<prefix>-<topic>`.
# Prefix list is built dynamically from categories.yaml at validate-time.


@dataclass
class Spec:
    raw: dict

    @property
    def categories(self) -> list[dict]:
        return self.raw["categories"]

    @property
    def forbidden_body_strings(self) -> list[dict]:
        return self.raw.get("forbidden_body_strings") or []

    @property
    def max_lines(self) -> int:
        return int(self.raw.get("max_skill_md_lines", DEFAULT_MAX_LINES))

    @property
    def max_bytes(self) -> int:
        return int(self.raw.get("max_skill_md_bytes", DEFAULT_MAX_BYTES))

    @property
    def max_description_bytes(self) -> int:
        """Cap on description length. 0 disables the check."""
        v = self.raw.get("max_description_bytes", DEFAULT_MAX_DESCRIPTION_BYTES)
        return int(v)

    @property
    def archive_path_components(self) -> list[str]:
        """Path components that mark a frozen-archive sub-tree. Any .md whose
        path contains one of these is skipped from the size-cap check, because
        archives are loaded by name on revisit rather than on trigger - the
        loader doesn't read them, so the cap doesn't earn its keep there.
        Default empty (no exemption). Example: ['results']."""
        v = self.raw.get("archive_path_components") or []
        return [str(p) for p in v]

    @property
    def prefixes(self) -> list[str]:
        return [c["prefix"] for c in self.categories if c.get("kind") == "prefix"]

    def match(self, skill_name: str) -> dict | None:
        # Exact-name match wins over prefix
        for cat in self.categories:
            if cat.get("kind") == "exact" and cat.get("exact") == skill_name:
                return cat
        # Longest-prefix match (so `ops-eng-sentry` wins over a hypothetical `ops-`)
        best = None
        best_len = -1
        for cat in self.categories:
            if cat.get("kind") != "prefix":
                continue
            p = cat["prefix"]
            if skill_name.startswith(p) and len(p) > best_len:
                best = cat
                best_len = len(p)
        return best


@dataclass
class Report:
    failures: list[str] = field(default_factory=list)

    def fail(self, msg: str) -> None:
        self.failures.append(msg)

    def ok(self) -> bool:
        return not self.failures

    def emit(self) -> None:
        for line in self.failures:
            sys.stderr.write(f"FAIL: {line}\n")


def load_spec() -> Spec:
    with SPEC_PATH.open(encoding="utf-8") as fh:
        data = yaml.safe_load(fh)
    return Spec(raw=data)


def parse_frontmatter(text: str) -> tuple[dict | None, str]:
    if not text.startswith("---\n"):
        return None, text
    end = text.find("\n---", 4)
    if end == -1:
        return None, text
    fm_raw = text[4:end]
    body_start = end + len("\n---")
    if text[body_start : body_start + 1] == "\n":
        body_start += 1
    try:
        fm = yaml.safe_load(fm_raw) or {}
    except yaml.YAMLError as exc:
        raise ValueError(f"frontmatter YAML parse error: {exc}") from exc
    if not isinstance(fm, dict):
        raise ValueError("frontmatter is not a YAML mapping")
    return fm, text[body_start:]


def extract_h2_sections(body: str) -> list[tuple[int, str]]:
    out: list[tuple[int, str]] = []
    in_code = False
    for i, line in enumerate(body.splitlines()):
        stripped = line.strip()
        if stripped.startswith("```"):
            in_code = not in_code
            continue
        if in_code:
            continue
        if stripped.startswith("## ") and not stripped.startswith("### "):
            out.append((i, stripped[3:].strip()))
    return out


def extract_h1_and_status(body: str) -> tuple[str | None, str | None, int]:
    lines = body.splitlines()
    h1 = None
    h1_idx = -1
    for i, line in enumerate(lines):
        stripped = line.strip()
        if stripped.startswith("# ") and not stripped.startswith("## "):
            h1 = stripped
            h1_idx = i
            break
    if h1 is None:
        return None, None, -1
    for j in range(h1_idx + 1, len(lines)):
        if lines[j].strip():
            return h1, lines[j].strip(), j
    return h1, None, -1


def normalize_section(name: str) -> str:
    return re.sub(r"\s+", " ", name).strip().lower()


def section_lead_line(body: str, section_name: str) -> str | None:
    target = normalize_section(section_name)
    in_code = False
    in_section = False
    for line in body.splitlines():
        stripped = line.strip()
        if stripped.startswith("```"):
            in_code = not in_code
            continue
        if in_code:
            continue
        if stripped.startswith("## ") and not stripped.startswith("### "):
            if normalize_section(stripped[3:]) == target:
                in_section = True
                continue
            elif in_section:
                return None
        elif in_section and stripped:
            return stripped
    return None


def check_forbidden_body_strings(
    md_path: Path, body: str, spec: Spec, report: Report
) -> None:
    rel = str(md_path.relative_to(REPO_ROOT))
    for entry in spec.forbidden_body_strings:
        s = entry["string"]
        allowed = entry.get("allowed_in") or []
        if rel in allowed:
            continue
        if s in body:
            line_no = body.split(s, 1)[0].count("\n") + 1
            report.fail(
                f"{md_path.relative_to(REPO_ROOT)}:{line_no}: forbidden body "
                f"string {s!r} found. This name belongs to a deleted skill. "
                f"Allowed only in: {allowed}"
            )


def check_stale_skill_refs(
    md_path: Path,
    body: str,
    current_skills: set[str],
    spec: Spec,
    report: Report,
) -> None:
    """Flag backticked references to non-existent skills under any known prefix.

    Prefix space can overlap natural language (e.g. `home-page` is a noun,
    `daily-driver` is an idiom), so this check only fires when the bare-backtick
    reference also appears in markdown-link form (`[text](path)`) elsewhere in
    the file. The markdown-link form is the intent signal that the author meant
    to cross-link a skill. The dead-link checker covers the navigable-link case
    directly; this catches the bare-backtick mention that drifts after a rename.
    """
    prefixes = spec.prefixes
    if not prefixes:
        return
    prefix_alt = "|".join(re.escape(p.rstrip("-")) for p in prefixes)
    backtick_pattern = re.compile(rf"`((?:{prefix_alt})-[a-z0-9][a-z0-9-]*?)(?:\.md)?`")
    # Names referenced via markdown link form anywhere in the body.
    link_targets: set[str] = set()
    for m in MD_LINK_RE.finditer(body):
        target = m.group(2) if m.lastindex and m.lastindex >= 2 else None
        if not target:
            continue
        # Non-file targets (URLs, anchors, mailto) are not local cross-links.
        if "://" in target or target.startswith(("#", "mailto:")):
            continue
        path_part = target.split("#", 1)[0]
        # A link that escapes the repo is a cross-repo link, not a local skill
        # cross-link, so it is not an intent signal for the stale-rename check.
        if not (md_path.parent / path_part).resolve().is_relative_to(REPO_ROOT):
            continue
        # Pull the basename for either source entry point, else the last segment.
        seg = path_part.rstrip("/").split("/")[-1]
        if seg in {"SKILL.md", "COMPOSED.md"}:
            parts = path_part.rstrip("/").split("/")
            if len(parts) >= 2:
                seg = parts[-2]
        link_targets.add(seg)
    rel = str(md_path.relative_to(REPO_ROOT))
    forbidden_allow_index = {
        e["string"]: set(e.get("allowed_in") or [])
        for e in spec.forbidden_body_strings
    }
    for m in backtick_pattern.finditer(body):
        ref = m.group(1)
        if ref in current_skills:
            continue
        if ref not in link_targets:
            continue  # only fire when the same name is also used as a link target
        if ref in forbidden_allow_index and rel in forbidden_allow_index[ref]:
            continue
        line_no = body.count("\n", 0, m.start()) + 1
        report.fail(
            f"{md_path.relative_to(REPO_ROOT)}:{line_no}: stale skill reference "
            f"`{ref}`. No such skill in the skills directory."
        )


def validate_skill(
    skill_dir: Path, spec: Spec, current_skills: set[str], report: Report
) -> None:
    name = skill_dir.name
    cat = spec.match(name)
    if cat is None:
        report.fail(
            f"skill {name!r} does not match an allowed prefix or exact-name. "
            f"Update categories.yaml first if you genuinely need a new shape."
        )
        return

    forbidden = cat.get("forbidden_names") or []
    if name in forbidden:
        report.fail(
            f"skill {name!r} is on the forbidden list for category "
            f"{cat['id']!r}. This name is reserved."
        )
        return

    skill_md = skill_dir / ENTRYPOINT_NAME
    if not skill_md.is_file():
        report.fail(f"skill {name!r} is missing {ENTRYPOINT_NAME}")
        return

    text = skill_md.read_text(encoding="utf-8")
    try:
        fm, body = parse_frontmatter(text)
    except ValueError as exc:
        report.fail(f"{name}/{ENTRYPOINT_NAME}: {exc}")
        return
    if fm is None:
        report.fail(f"{name}/{ENTRYPOINT_NAME}: missing or malformed YAML frontmatter")
        return

    # Repo-pointer skills are generated and owned by the repo-pointer-skills hook,
    # so skip shape validation here. Global checks still apply via run_global_checks.
    if cat.get("role") == "repo-pointer":
        return

    fm_name = fm.get("name")
    if fm_name != name:
        report.fail(
            f"{name}/{ENTRYPOINT_NAME}: frontmatter `name: {fm_name!r}` does not match "
            f"directory name {name!r}"
        )
    description = fm.get("description")
    if not description or not str(description).strip():
        report.fail(f"{name}/{ENTRYPOINT_NAME}: frontmatter `description` is empty")
        description = ""
    description = str(description).strip()

    if spec.max_description_bytes > 0:
        desc_bytes = len(description.encode("utf-8"))
        # Router and meta categories get 2x the cap. They genuinely need more
        # keyword surface to fan out to the skills they route across.
        role = cat.get("role")
        effective_cap = spec.max_description_bytes
        if role in {"router", "meta"}:
            effective_cap *= 2
        if desc_bytes > effective_cap:
            role_note = " (2x router/meta cap)" if role in {"router", "meta"} else ""
            report.fail(
                f"{name}/{ENTRYPOINT_NAME}: description is {desc_bytes} bytes, over the "
                f"{effective_cap}-byte cap{role_note}. Every skill's description "
                f"is loaded into every agent session, so descriptions are pure "
                f"context burn. Tighten by moving procedure into the body, dropping "
                f"redundant aliases, or splitting the skill."
            )

    desc_pat = cat.get("description_prefix_pattern")
    desc_exceptions = set(cat.get("description_prefix_exceptions") or [])
    if desc_pat and name not in desc_exceptions:
        head = re.sub(r"\s+", " ", description).strip()
        if not re.match(desc_pat, head):
            report.fail(
                f"{name}/{ENTRYPOINT_NAME}: description does not match required prefix "
                f"for category {cat['id']!r}. Pattern: {desc_pat!r}. "
                f"Description starts: {head[:100]!r}"
            )

    if not cat.get("enforce_status"):
        return

    h1, status_line, _ = extract_h1_and_status(body)
    if h1 is None:
        report.fail(f"{name}/{ENTRYPOINT_NAME}: missing H1 heading")
        return
    if status_line is None:
        report.fail(f"{name}/{ENTRYPOINT_NAME}: missing status line under the H1")
        return

    m = STATUS_LINE_RE.match(status_line)
    if not m:
        report.fail(
            f"{name}/{ENTRYPOINT_NAME}: status line does not match required format. "
            f"Expected: 'Status: <emoji> <Kind> | Last <updated|tested>: YYYY-MM-DD'. "
            f"Got: {status_line!r}"
        )
        return

    emoji = m.group("emoji")
    kind = m.group("kind")
    freshness = m.group("freshness")

    expected_freshness = cat.get("status_freshness")
    if freshness != expected_freshness:
        report.fail(
            f"{name}/{ENTRYPOINT_NAME}: status line uses 'Last {freshness}' but category "
            f"{cat['id']!r} requires 'Last {expected_freshness}'"
        )

    status_kinds = cat.get("status_kinds") or {}
    if kind not in status_kinds:
        report.fail(
            f"{name}/{ENTRYPOINT_NAME}: status kind {kind!r} not in allowed kinds for "
            f"category {cat['id']!r}: {sorted(status_kinds)}"
        )
        return
    expected_emoji = status_kinds[kind]
    if emoji != expected_emoji:
        report.fail(
            f"{name}/{ENTRYPOINT_NAME}: status emoji {emoji!r} does not match required "
            f"{expected_emoji!r} for kind {kind!r} (category {cat['id']!r})"
        )

    h1_by_status = cat.get("h1_pattern_by_status") or {}
    h1_pat = h1_by_status.get(kind)
    if h1_pat and not re.match(h1_pat, h1):
        report.fail(
            f"{name}/{ENTRYPOINT_NAME}: H1 {h1!r} does not match required pattern "
            f"{h1_pat!r} for status kind {kind!r}"
        )

    required_by_status = (cat.get("required_sections") or {}).get("by_status") or {}
    required = required_by_status.get(kind, [])
    h2s = extract_h2_sections(body)
    present = {normalize_section(n) for _, n in h2s}
    if required:
        missing = [s for s in required if normalize_section(s) not in present]
        if missing:
            report.fail(
                f"{name}/{ENTRYPOINT_NAME}: missing required sections for Status: {kind} "
                f"-> {missing}"
            )

    lead_specs = cat.get("section_lead_lines") or {}
    for section_name, lead_pat in lead_specs.items():
        if normalize_section(section_name) not in present:
            continue
        lead = section_lead_line(body, section_name)
        if lead is None or not re.match(lead_pat, lead):
            report.fail(
                f"{name}/{ENTRYPOINT_NAME}: section '## {section_name}' first line "
                f"does not match required pattern {lead_pat!r}. Got: {lead!r}"
            )


def check_size_caps(
    md_path: Path, spec: Spec, report: Report, role: str | None = None
) -> None:
    # Frozen-archive exemption: loaded by name on revisit, not by trigger.
    archive_parts = set(spec.archive_path_components)
    if archive_parts and archive_parts.intersection(md_path.parts):
        return
    if role == "thin":
        max_lines, max_bytes = THIN_MAX_LINES, THIN_MAX_BYTES
        cap_note = " (role: thin, 1/4 cap)"
    else:
        max_lines, max_bytes = spec.max_lines, spec.max_bytes
        cap_note = ""
    n_lines = sum(1 for _ in md_path.open(encoding="utf-8"))
    if n_lines > max_lines:
        report.fail(
            f"{md_path.relative_to(REPO_ROOT)}: {n_lines} lines exceeds the "
            f"{max_lines}-line cap{cap_note}. Move detail into a sibling "
            f"references/ file."
        )
    n_bytes = md_path.stat().st_size
    if n_bytes > max_bytes:
        report.fail(
            f"{md_path.relative_to(REPO_ROOT)}: {n_bytes} bytes exceeds the "
            f"{max_bytes}-byte cap{cap_note}. Move detail into a sibling "
            f"references/ file."
        )


def gather_current_skills(extra_dirs: list[Path] | None = None) -> set[str]:
    out = set()
    for root in [SKILLS_DIR, *(extra_dirs or [])]:
        if not root.is_dir():
            continue
        for p in root.iterdir():
            if p.name.startswith("."):
                continue
            if p.is_symlink():
                # Symlinks are skipped from validation but remain known names.
                out.add(p.name)
                continue
            if p.is_dir():
                out.add(p.name)
    return out


def iter_skill_dirs() -> list[Path]:
    return sorted(
        p
        for p in SKILLS_DIR.iterdir()
        if p.is_dir() and not p.is_symlink() and not p.name.startswith(".")
    )


def run_global_checks(
    spec: Spec,
    current_skills: set[str],
    selected: set[str] | None,
    report: Report,
) -> None:
    for entry in iter_skill_dirs():
        if selected is not None and entry.name not in selected:
            continue
        skill_md = entry / ENTRYPOINT_NAME
        if not skill_md.is_file():
            continue
        body = skill_md.read_text(encoding="utf-8")
        cat = spec.match(entry.name)
        role = cat.get("role") if cat else None
        check_forbidden_body_strings(skill_md, body, spec, report)
        check_stale_skill_refs(skill_md, body, current_skills, spec, report)
        check_size_caps(skill_md, spec, report, role=role)


def main(argv: list[str] | None = None, hook_id: str = HOOK_ID) -> int:
    if not is_enabled(hook_id):
        print(f"{hook_id}: disabled by repo config")
        return 0
    if argv is None:
        argv = sys.argv

    parser = argparse.ArgumentParser(
        prog="check-skills",
        description="Validate the structure of a repo's skill directory.",
    )
    parser.add_argument(
        "--skills-dir",
        default=None,
        help="Path to the skills directory (relative to the repo root). "
        "Default: autodetect .agents/skills, .claude/skills, skills.",
    )
    parser.add_argument(
        "--entrypoint",
        choices=("SKILL.md", "COMPOSED.md"),
        default="SKILL.md",
        help="Entrypoint filename required in each skill directory.",
    )
    parser.add_argument(
        "--spec-path",
        default=None,
        help="Category spec path relative to the repo root. Defaults to categories.yaml under --skills-dir.",
    )
    parser.add_argument(
        "--reference-skills-dir",
        action="append",
        default=[],
        help="Additional skill root whose names satisfy cross-references.",
    )
    parser.add_argument(
        "--report-only",
        action="store_true",
        help="Exit 0 even if there are failures (the report still prints).",
    )
    parser.add_argument(
        "names",
        nargs="*",
        help="Optional list of skill names to validate. When empty, all "
        "skills are validated.",
    )
    ns = parser.parse_args(argv[1:])

    # Mutate the module globals so the rest of the validator's call graph
    # (which references SKILLS_DIR / SPEC_PATH directly) sees the override.
    global SKILLS_DIR, SPEC_PATH, ENTRYPOINT_NAME
    SKILLS_DIR = (REPO_ROOT / (ns.skills_dir or detect_skills_dir())).resolve()
    SPEC_PATH = (
        (REPO_ROOT / ns.spec_path).resolve()
        if ns.spec_path
        else SKILLS_DIR / "categories.yaml"
    )
    ENTRYPOINT_NAME = ns.entrypoint

    if not SKILLS_DIR.is_dir():
        # No-op for repos without a skills surface.
        return 0
    if not SPEC_PATH.is_file():
        # No-op for a partial skills surface lacking categories.yaml.
        return 0

    report_only = ns.report_only
    args = ns.names

    spec = load_spec()
    report = Report()
    reference_dirs = [
        (REPO_ROOT / path).resolve() for path in ns.reference_skills_dir
    ]
    current_skills = gather_current_skills(reference_dirs)

    selected = set(args) if args else None

    for entry in iter_skill_dirs():
        if selected is not None and entry.name not in selected:
            continue
        validate_skill(entry, spec, current_skills, report)

    run_global_checks(spec, current_skills, selected, report)

    if report.ok():
        print("skill conventions: OK")
        return 0
    report.emit()
    sys.stderr.write(f"\n{len(report.failures)} violation(s).\n")
    return 0 if report_only else 1


if __name__ == "__main__":
    sys.exit(main())
