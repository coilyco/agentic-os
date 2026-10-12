# Where the docs live in code

Not loose block comments scattered in the source. One markdown file per documentation surface, embedded into the binary and emitted by the CLI.

For a Go CLI, `//go:embed` a markdown file per command. The file stays editable as plain markdown, legible to a human reviewing it, while the CLI owns delivery and timing.

**Why:** the markdown file versions in the same commit as the command code. A skill markdown describing command behavior is a second copy of the truth that rots on its own. Embedded-and-emitted docs cannot drift from the command, because changing the command and changing its docs is one diff.

**How to apply:** new agent-facing command - author `intro.md`, `help.md`, `outro.md` (or one file with delimited sections), `//go:embed` them, emit Intro and Outro from the command body and Help from a `help` subcommand. Existing command with docs in a skill - move the behavioral half into embedded files on next touch, leave only Description and Body in the skill.

## What stays in the skill

After the move, the skill shrinks to tiers 1 and 2 - Description and Body. That is the part that is genuinely human-to-agent and genuinely fuzzy-matched. Everything past Body is agent-parsed reference that belongs next to the code.

The Body ends with a one-liner: "for command behavior, run `<tool> help`."

## Worked example: the retired Ward launcher

Ward's agent launcher was the command that took this shape, before Ward retired (`COI-2727`). Its behavioral reference - work vs headless vs task, the pre-flight GO/NO-GO read, reservation, `--new-tab`, refusal conditions - lives in `docs/agent.md` next to the code (embedded and emitted), not in a skill. A skill would only hold tiers 1 and 2 - the Description and Body that are genuinely human-to-agent and fuzzy-matched - and end with a pointer to the command's own help. The retired `ward dispatch` was the first command to motivate this split (ward#174).
