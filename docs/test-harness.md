# Test harnesses

## Test harnesses

A **test harness** probes one agent and model pairing before it is trusted with real work: what the agent claims about itself versus what it can actually do. Each documented probe gets its own `test-harness-<agent>` page. These probes are evidence, not production routing defaults.

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
- **Related** - the explicit launch path, runtime configuration, and this parent.

Keep it public-safe: no tower FQDN or opaque ids (use placeholders, resolve at runtime). Findings are point-in-time - date the model/version they were taken against.

## Index

- [test-harness-composed-roles](test-harness-composed-roles.md) - authenticated
  Codex plus the thirteen-role local Goose question matrix. **Landed.**
- [test-harness-goose](test-harness.md) - Goose driving `qwen3-coder:30b` on the tower. **Landed.**
- `test-harness-codex` - Codex on `gpt-5.5` (ChatGPT auth). Authored by Codex.
- `test-harness-opencode`, `test-harness-aider`, `test-harness-claude` - planned.

Goose is the only currently proven local-model production launch. Other entries
remain exploratory until their own evidence supports production use.

## Test harness: Goose

Part of the [test-harness](test-harness.md) doc family - one per agent.

`scripts/goose-ask.sh` (ward verb `goose-ask`) is a minimal probe harness for
Goose. It runs one-shot questions through
`goose run --no-session`, strips the startup banner, times each call, and tees
a full raw transcript under
`~/.cache/agentic-os/goose-ask/<timestamp>.log`. The goal is to interrogate a
Goose+model pairing before trusting it with real work.

The standalone `goose-health` probe was not kept here. Its tower/Ollama reachability and residency checks belong in the always-on agent-health heartbeat in `infrastructure`, which is the canonical place for the serving-layer view.

## Mechanism

`goose run --no-session -t "<question>"` against the bound model in `~/.config/goose/config.yaml` (currently `qwen3-coder:30b` via Ollama on the tower over tailnet). `goose-ask.sh` wraps that with banner stripping, timing, transcript capture, and `-f` batch / `-s` system / `-m` model-override flags.

## Usage

```
ward exec goose-ask -- "a single question"
ward exec goose-ask -- -f scripts/goose-probe.txt        # capability battery
ward exec goose-ask -- -m qwen3:30b-a3b "compare models" # override GOOSE_MODEL
ward exec goose-ask -- -s "Answer in one word." "ping"   # extra system text
```

ward gates repo verbs on a clean working tree, so run the raw script (`bash scripts/goose-ask.sh ...`) while iterating on uncommitted changes. Each question is an independent `--no-session` run, so no state carries between them. Clean answers print to stdout (tool-call rounds included, since they are part of what Goose actually did); the raw log is the source of truth.

`scripts/goose-probe.txt` is the capability battery: it probes what the model *claims* about itself (often wrong) and what the `developer` extension can *actually* do (shell and file tools, exercised live).

## First prod-test findings (qwen3-coder:30b on the tower)

Run against `qwen3-coder:30b` via Ollama on `kai-tower-3026` over tailnet, Goose 1.38.0. These shape any heartbeat / triage loop built on this pairing:

- **Tool use is solid.** `shell`, `tree`, `analyze`, `write`, `edit`, `load_skill`, `todo_write`, `apps__*`, `extensionmanager__*` all fire and return real output. The shell probe returned verbatim correct `pwd`/`whoami`/`uname`, and the working directory is correctly the invocation cwd. Goose 1.38 ships more builtins (`analyze`, `apps`, `extensionmanager`, `skills`) than the lone `developer` entry in `config.yaml` implies.
- **It over-iterates badly.** A one-line identity question burned 24s and ~8 redundant tool calls (compulsive `todo_write`, repeated empty `echo $GOOSE_MODEL` shells). For a triage loop this is the main risk: it thrashes instead of answering. Mitigate with a recipe that fixes the output shape, a tight system prompt, and disabling the todo extension for short tasks.
- **Self-knowledge is wrong but plausible.** It claims "Qwen3 30B-A3B, hosted locally on your machine" - the model cannot see that it is `qwen3-coder:30b` on a remote tower. Never trust the model's self-report for routing; read it from config.
- **Brevity instructions are weak.** "In one line" / "in one sentence" are routinely ignored. Constrain output with structure (a recipe schema), not politeness.
- **Context window is hazy.** It infers 128k from the live context meter; the model file actually advertises 256k. Goose's effective `num_ctx` is the real ceiling, set it explicitly rather than asking the model.

## Related

- [test-harness](test-harness.md) - the parent doc family and authoring convention.
- `models-qwen-coder` - the bound model tier.
- [AOS launch CLI](aos-cli.md) - selecting Goose and a role explicitly.
