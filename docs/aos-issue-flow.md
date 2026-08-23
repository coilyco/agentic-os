# AOS issue flow

How a session picks up an issue and hands it back.

## AOS composed-role check-in

`acompose-checkin` asks a composed role to identify and describe itself without
requiring the operator to repeat an agent's non-interactive invocation policy.

## Invocation

```bash
aos --role platform --agent codex acompose-checkin
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

## AOS issue-pin context

AOS can hydrate a bounded set of Forgejo pinned issues into launch context for
one selected role. Agent Compose receives only local files. It never performs
the Forgejo read.

## Configuration

Place the generic config at `.agents/issue-pin-context.yaml`, or point
`AOS_ISSUE_PIN_CONTEXT` at another file:

```yaml
roles:
  exec:
    forgejo:
      owner: coilysiren
      repo: inbox
      collection: pinned
      max_bytes: 24576
      freshness: 15m
      fail_closed: true
```

`base_url` defaults to `https://forgejo.coilysiren.me`. `collection` currently
supports `pinned`. Kai-specific role-to-repo selection belongs in AOSK or local
operator config, not in the AOS product default.

## Runtime

At launch, AOS reads the role config, fetches pinned issues when the cache is
missing or stale, and writes a local Markdown context file. Standalone
containers receive that file at `/run/aos/issue-pins.md`. Warded launches mount
the same file into context-bundle materialization.

The rendered context preserves pin order, canonical URL, issue number and title,
exact body text, issue `updated_at`, hydration time, source repository, and a
snapshot digest. If `max_bytes` prevents full body inclusion, metadata remains
and the body carries an explicit clipped marker.

The context is current-state evidence only. It grants no permissions, tools,
credentials, mounts, repository access, or workflow authority.

## Cache And Freshness

Snapshots live under the AOS user cache, or under `AOS_ISSUE_PIN_CACHE_DIR` when
set. A fresh verified snapshot is reused without a Forgejo request. A stale
snapshot is marked with its age when live refresh fails and `fail_closed` is
false. With `fail_closed: true`, a stale or unavailable live read stops launch.

`FORGEJO_TOKEN` or `AOS_FORGEJO_TOKEN` may authenticate the host-side read. The
token is never written into the snapshot, generated Markdown, context bundle, or
container mount.
