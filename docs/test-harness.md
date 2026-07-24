# Test harnesses

A **test harness** probes one agent+model pairing before it is trusted with real work: what the agent *claims* about itself (often wrong) versus what it can *actually* do (tool use, file/shell reach, instruction-following). Each of the five harnesses in [harness-selection.md](harness-selection.md) gets its own doc in this family, named `test-harness-<agent>`.

This is the parent. Author one child per agent.

## Why per-agent

The probing *mechanism* differs by harness, so the doc does too:

- **Goose** - a wrapper script (`scripts/goose-ask.sh`, ward verb `goose-ask`) runs one-shot questions through `goose run --no-session`.
- **Codex** - `codex exec` invocations (note `--skip-git-repo-check` is needed outside a trusted git repo).
- **OpenCode / Aider** - the harness's own non-interactive run mode against the bound local model.
- **Claude** - cloud, large context, semantic skill-selection - probed differently again.

What stays constant is the *questions* (the probe battery) and the *findings shape* (below), so harnesses can be compared on equal terms.

## Authoring a new harness doc

Name it `test-harness-<agent>.md`, link it from this parent's index, and follow these sections:

- **Mechanism** - exactly how to invoke this agent for one-shot probing, with the gotchas (auth, flags, host).
- **Usage** - the concrete commands, single and batch.
- **First prod-test findings** - run the probe battery once and record what it surfaced, organized by: tool use (what fires, what returns real output), iteration discipline (does it thrash), self-knowledge (model/host self-report accuracy), instruction-following (brevity, format), context window (claimed vs real).
- **Related** - the agent profile skill, the bound model tier, this parent.

Keep it public-safe: no tower FQDN or opaque ids (use placeholders, resolve at runtime). Findings are point-in-time - date the model/version they were taken against.

## Index

- [test-harness-composed-roles](test-harness-composed-roles.md) - authenticated
  Codex plus the ten-role local Goose question matrix. **Landed.**
- [test-harness-goose](test-harness-goose.md) - Goose driving `qwen3-coder:30b` on the tower. **Landed.**
- `test-harness-codex` - Codex on `gpt-5.5` (ChatGPT auth). Authored by Codex.
- `test-harness-opencode`, `test-harness-aider`, `test-harness-claude` - planned.

## Related

- [harness-selection.md](harness-selection.md) - picking a harness and model tier.
- [agents-goose](../.agents/skills/agents-goose/SKILL.md), [agents-claude](../.agents/skills/agents-claude/SKILL.md) and siblings - the harness profiles.
