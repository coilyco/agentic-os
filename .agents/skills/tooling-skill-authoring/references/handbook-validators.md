# Handbook: Validators and pre-commit wiring

## 6. Validators

The structural validator and dead-link checker ship from [`coilyco-flight-deck/agentic-os`](https://github.com/coilyco-flight-deck/agentic-os) and are consumed via pre-commit. The em-dash check is a small local hook because the upstream is voice-neutral by design.

### Documentation-wide validators

`catalog-trifecta`, `catalog-doc-size`, `documentation-layout`, and `code-comments` apply beyond skills. Together they enforce the doc surface shape:

* `README.md`, `AGENTS.md`, and `docs/FEATURES.md` exist, cross-link, and stay under the size caps in [`check_documentation_layout.py`](../../../../agentic_os/pre_commit/check_documentation_layout.py).
* `AGENTS.md` uses the standard repo-local H2 set.
* Markdown lives only at repo root, flat `docs/*.md`, or flat ordinary and
  composed source folders. Every Markdown file stays under the cap.
* Code comments are up to two contiguous lines, max 90 chars each, with long explanation moved to docs. YAML allows just one comment line, only as the first line, so a key-sorter cannot drift it off its target.

### `skill-conventions` (upstream) - structural check

Reads `.agents/skills/categories.yaml`, walks `.agents/skills/`, applies all checks, exits non-zero with a per-violation report.

What it checks:

1. **Skill prefix.** Every directory under `.agents/skills/` matches an allowed prefix or exact name.
2. **SKILL.md exists.**
3. **Frontmatter valid.** Has `name` (equal to dir name) and non-empty `description`.
4. **Description prefix** (when enforced per category). Optional. Most categories leave this off.
5. **Forbidden directory names** (per category, when set).
6. **Forbidden body strings** (global allow-list).
7. **Status line** (when enforced). Format `Status: <emoji> <Kind> | Last <updated|tested>: YYYY-MM-DD`. Kind in allowed list, emoji matches the kind's required pairing.
8. **H1 pattern** (when enforced).
9. **Required H2 sections** (when enforced). Dispatched by Status kind.
10. **Section lead lines** (when enforced).
11. **Stale skill-name backtick references.** Catches `` `<prefix>-<topic>` `` references whose target skill doesn't exist.
12. **SKILL.md size caps.** Cap in [`check_documentation_layout.py`](../../../../agentic_os/pre_commit/check_documentation_layout.py), same as all Markdown. Push detail into a sibling `<topic>.md` file.
13. **Symlinks under `.agents/skills/`.** Symlink dirs are skipped, not validated. The loader follows them, and the validator walks the canonical target.

### `check-composed-skills` (upstream) - role isolation

Reads the ordinary category spec and walks `.agents/composed/`. It requires
`COMPOSED.md`, applies the content checks above, and rejects any `SKILL.md`,
symlink, non-directory entry, or name shared with `.agents/skills/`.

### `dead-cross-links` (upstream) - cross-link check

Walks every Markdown file in the repo, including both skill source roots,
extracts inline links, and fails on any local-relative target that does not
resolve.

What it skips intentionally:

* External URLs (`http://`, `https://`, `mailto:`, etc).
* Paths escaping the repo via `../` (treated as external).
* Anchors (`#section`) and placeholder targets (`...`, `TBD`, `TODO`).
* Inside fenced code blocks.
* Files named `TEMPLATE.md`.

### `em-dash-check` (local) - voice rule

`scripts/check-em-dashes.py` flags U+2014 in SKILL.md prose, masking inline code, fenced code, quoted strings, and link targets first. Stays local because the upstream validator is voice-neutral and em-dashes are a personal preference, not a general convention.

### Pre-commit wiring

`.pre-commit-config.yaml` subscribes to `coilyco-flight-deck/agentic-os` at a pinned tag for `skill-conventions` and `dead-cross-links`. The four local hooks (`trufflehog`, `leak-check`, `em-dash-check`, `setup-symlinks`) stay as `repo: local` entries.

Bump the `rev:` to pull upstream changes. Add new local checks as new `repo: local` hook entries.
