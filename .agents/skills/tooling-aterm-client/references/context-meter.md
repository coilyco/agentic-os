# The context meter

Every seat shows a context-token count in the same place, the right side of the
session header, whatever its harness. Kai named it as an exception to the web
client freeze on 2026-10-04 (agentic-os PR 1818), so the freeze holds for
everything else.

## What it shows

`<count> tokens`, and `// <n>% of <window>` only where the source knows the
model's window. A small count in a large window reads `<1%`, never `0%`. No
reading yet means no pill, since a bare `0` would claim an empty context.

## Where the daemon reads it

`sessionView.context` carries `tokens`, `window` and `source`, refreshed every
4 seconds, pushed on the `sessions` channel when a reading moves, and advertised
as the `context` welcome feature. `aterm/context.go` owns it.

* **claude** - the latest main-thread assistant line in the transcript the ledger
  finds, summing fresh, cached and newly cached input plus output. A subagent
  line and a synthetic line are skipped. The transcript records no window, so
  `window` is set only when the launch argv has `--model ...[1m]`. A seat launched
  without `--session-id` has no transcript to find and shows no pill.
* **codex** - the latest `token_count` event with an info block in the rollout,
  `last_token_usage.total_tokens` over `model_context_window`.
* **every other seat** - Agent Proxy's `GET /v1/sessions/usage` at
  `ATERM_AGENT_PROXY_URL`, which the daemon reads from `daemon.env` like its other
  `ATERM_` settings. Unset turns the source off. A reading older than the session
  is dropped, since a resume reuses a name.

Only the last 2 MB of a transcript is read, and only when its size or time moved.
A failed round keeps the last reading and logs once.

## How a proxy seat gets counted

The seat must send its session name as `x-agent-session-id`, which Agent Proxy
keys its rollup by. `aos` sets it at launch from `ATERM_SESSION`
(`aos-cli/native_session_header.go`), and says on stderr when it cannot.

* **goose** - `OPENAI_CUSTOM_HEADERS`, but goose reads it from env only when
  `OPENAI_API_KEY` is in env too, so a goose seat whose key lives in goose's own
  secret store sends no header.
* **opencode** - a config file holding only `provider.agent-proxy.options.headers`,
  pointed at by `OPENCODE_CONFIG`. A caller's own `OPENCODE_CONFIG` is left alone.

## The seam

`contextSource` in `aterm/context.go` is the next harness's reader. The client
reads `Session.context` and nothing else, so a new source needs no client change.
