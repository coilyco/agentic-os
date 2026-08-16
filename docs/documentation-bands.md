# Documentation size bands

## Documentation size bands

A repo declares one band and gets its three caps: lines and chars per Markdown
file, and how many `docs/*.md` files it may carry.

| band | lines | chars | docs |
| --- | --- | --- | --- |
| `small` (default) | 40 | 3,000 | 20 |
| `large` | 120 | 8,000 | 40 |

Declared under the hook config. No declaration means `small`, so a repo opts up
deliberately rather than drifting up.

```toml
[tool.agentic-os.documentation-layout]
band = "large"
```

## Why a count cap exists at all

A per-doc size cap does not bound a docs folder. It reshapes it. A repo that
caps length and not count answers every over-long doc by splitting it, and the
folder grows without any single file ever failing.

`sirens-echo` is the proof: 156 docs, median 2,935 chars, largest 3,989 against
a 4,000 cap. Not one file over the limit, and a folder nobody can read.

## Why lines bind before chars

Measured across the fleet, Markdown here runs about 49 characters per line. So
40 lines is roughly 1,960 characters and 120 lines is roughly 5,880. The char
cap sits above both on purpose: it is the backstop that catches a doc dense
with tables or code, not the everyday constraint.

That relationship is what makes the pair work. A char cap set near the line
cap's natural size would fire constantly on ordinary prose; one set far above
it would never fire at all.

## The two caps multiply

Count times lines is a total documentation budget, and it is the number worth
arguing about rather than either cap alone.

* `small` - 20 x 40 = 800 lines
* `large` - 40 x 120 = 4,800 lines

A repo cannot escape the budget by trading one cap against the other. Merging
two docs to clear the count spends the line cap, and splitting one to clear the
line cap spends the count.

## No per-file escape

`excludes` still governs placement and flatness, and no longer reaches either
size cap or the count. A generated file that lands over the cap is a generator
emitting too much, and the fix is the generator.

The rule this replaced allowed a per-file exemption, and the exemptions
accumulated exactly where the pressure was highest: one repo excluded nine
individual `SKILL.md` files, two excluded `docs/FEATURES.md` from the cap that
exists to keep it an inventory.

## Documentation-layout exceptions

Every carve-out from the default `documentation-layout` rule: Markdown lives only at the root allowlist, `docs/*.md` (flat), or a skill dir, capped at 80 lines / 4000 chars. Enforced by [`check_documentation_layout.py`](https://github.com/coilyco-flight-deck/agentic-os/blob/main/agentic_os/check_documentation_layout.py); per-repo config under `[tool.agentic-os.documentation-layout]` in `pyproject.toml`. File lists are a snapshot and drift; the mechanisms do not.

## Active in this repo

- **Gitignored paths** - skipped unasked. See [build output is not content](build-output-is-not-content.md).
- **`excludes` (config)** - fully exempt from location AND size checks, per the layout escape hatch. Today: `warp/launch_configurations/**` (README) and `warp/tab_configs/**` (wtab.md, claude-agent-work.md). This is the one hand-maintained surface, so it drifts - prune it when a file moves.
- **AGENTS.md size override** - config keys `agents_md_max_lines = 290` and `agents_md_max_chars = 26000` replace the shared 320/25000 default for `AGENTS.md` only. Lowered from 320/34000 once role-scoped doctrine moved to the agent-compose role melds that own it. The remaining room holds universal doctrine that must stay in-context, and the margin is one section wide on purpose.
- **FEATURES.md size cap** - `docs/FEATURES.md` uses the tight inventory cap from `check_documentation_layout.py`: 80 lines / 4000 chars. That keeps it a major-capability index, not a changelog.
- **Root allowlist** - present: `AGENTS.md`, `CLAUDE.md`, `CODE-REVIEW.md`, `README.md`. Any other root `*.md` fails the location check and would need either a move into `docs/` or an explicit exclude.
- **Skill-path location carve-out** - `*.md` under `.agents/skills/`, `.claude/skills/`, or `skills/` may live outside `docs/` at any depth (~220 files here). Location-only: the 80/4000 size cap still applies.

## Dormant carve-outs

These ship in the shared validator for other repos in the family but match nothing here:

- **`SIZE_CAP_EXEMPT_BASENAMES`** - `CODE_OF_CONDUCT.md` is exempt from the size cap by basename (verbatim-upstream file). Not present.
- **`examples/` carve-out** - any `*.md` under an `examples/` dir is allowed at any depth (Go/Rust idiom). No such file here.
- **README.md size opt-up** - config keys `readme_max_lines` / `readme_max_chars` lift the root `README.md` past the overview default (160 / 12500), the same mechanism as the AGENTS.md override. Unset here, so this repo's README rides the overview cap. Used by a release repo whose README is the launch-grade front page.

## Suppressed wholesale

- **`SKIP_DIR_NAMES`** - never scanned at all: `.git`, `.claude`, `.venv`, `.mypy_cache`, `.pytest_cache`, `.ruff_cache`, `.terraform`, `.tox`, `__pycache__`, `build`, `dist`, `node_modules`, `target`, `vendor`.

## Baseline (not an exception)

- `docs/` must stay flat - no subdirectories, use filename prefixes (`features-*.md`, `warp-*.md`, `skill-discipline-*.md`).
- A nested `SKILL.md` below a top-level skill dir fails: the loader only sees top-level dirs.
- A co-located **module `README.md`** is allowed in one of two capped shapes, each <= 3 non-blank lines (blank lines free, prose lines <= 90 chars). **Outpost** - a heading, optional one-sentence summary, and exactly one link to a single `docs/*.md` file that must link back to that exact README path (reciprocal, file-to-file). One doc may anchor many outposts. **Homestead** - heading plus up to 2 content lines, no `docs/` pointer (self-contained signage). The discriminator is whether the README links a `docs/*.md` file. This turns the per-repo `ansible/README.md` / `deploy/*/README.md` excludes into a rule - a conforming README needs no exclude.

## See also

- [README.md](../README.md) - repo intro.
- [warp.md](warp.md) - an example of the docs/ prefix split (`warp.md` + `warp-host-setup.md`).
