# The five tiers

Every agent-facing CLI command has five distinct information surfaces. They differ by who reads them and when.

1. **Description** - match time. Skill frontmatter `description:`. Aliases and triggers. The only surface doing fuzzy-matching work. Eager-loaded into every turn, so it stays under the validator's `max_description_bytes` cap (public default 500, agentic-os-kai 200, documented in the skill-discipline docs in agentic-os).
2. **Body** - orient time. Skill body. Read after the description matched but before the agent commits to running anything. Three to five lines: what the command does, its blast radius, and a pointer to `help`. Not a bare "aliases for `ward X`" line.
3. **Intro** - pre-run, pushed. CLI-emitted at the start of a real run. Short, two to three lines, because it hits the agent's context on every single invocation. Ends with "run `ward X help` for the full thing."
4. **Help** - pre-run, pulled. CLI-emitted on demand via `ward X help`. The exhaustive long-form reference. Safe to read with no side effects.
5. **Outro** - post-run, pushed. CLI-emitted at completion. The next-action nudge ("dispatch finished, run `ward session end`, merge back to main").
