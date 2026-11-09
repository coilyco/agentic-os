# AOS issue flow

The composed-role check-in. The issue-pin context that once shared this page was deleted with Forgejo (COI-3150).

## AOS composed-role check-in

`acompose-checkin` asks a composed role to identify and describe itself without
requiring the operator to repeat an agent's non-interactive invocation policy.

## Invocation

```bash
aos --role eng-platform --agent codex acompose-checkin
```

`--role` selects the agent-compose role. `--agent` selects the executable
adapter and its defaults. Codex is the first supported adapter.

## Codex adapter

The adapter runs `codex exec` with an ephemeral session, a read-only sandbox,
no color, and no Git-repository requirement. The container supplies its
Terra-medium defaults and stages validated file-backed or macOS Keychain Codex
auth through the normal AOS path.

The check-in skips the general substrate while retaining the AOS provider
required to compose the role. A conflicting explicit `--layout` fails instead
of projecting one harness and executing another.

The prompt forbids tool use and asks the agent to begin with
`ROLE-CONFIRMED: <role>`, then describe itself in under 180 words. The CLI
prints the response without interpreting it. The role-question harness remains
the assertion path for an automated pass or fail.

The normal standalone bootstrap still projects the host MCP inventory and
tailnet bridge. The check-in prompt deliberately leaves those tools unused, so
a successful role confirmation proves identity composition rather than MCP
health or tailnet reachability.

The diagnostic transcript stays streaming in emission order. A duplicated
final stdout copy is suppressed. Blank lines frame the transcript, each Codex
section divider, and each prompt, warning, response, or token block.

## Inspection

Global `--image`, `--delivery`, `--auth`, and `--dry-run` behavior still
applies. A dry run renders the Docker launch without exposing auth or forwarded
environment values. The default `--auth=true` requires readable supported
Codex auth before Docker starts. `--auth=false` is reserved for deliberate
unauthenticated commands and does not turn a check-in into an inference proof.
