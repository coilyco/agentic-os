# AOS convergence and the catalogue cache

AOS owns host-aware agent inputs. `aos converge` hydrates remote skill
catalogues, emits their local manifest, and projects one mcporter inventory
into native harness configuration. Agent Compose consumes local inputs after
this boundary.

## Configuration

The default source is `~/.config/aos/converge.yaml`. An explicit `--config`
path selects another source. Relative paths resolve from its directory.
`${HOME}` and `~/` resolve against `--home` or the current user's home.

```yaml
catalogues:
  state_dir: ~/.local/state/aos
  manifest: ~/.config/aos/catalogues.json
  cache_ttl: 10m
  sources:
    - https://example.test/owner/catalogue.git/.agents/skills@main

mcp:
  inventory: ~/.config/mcporter/mcporter.json
  project_native: true
```

The sections are independent. An absent default config is an intentional
no-op. An explicitly selected missing config is an error.

## MCP contract

Canonical inventories set `imports` to an empty list. For compatibility with
older inventories, AOS treats an absent key as empty and writes an explicit
`imports: []` value into the projected inventory. Malformed or non-empty
imports remain an error. AOS always copies the safe inventory to
`~/.mcporter/mcporter.json`. With `project_native: true`, AOS also:

* replaces Claude's `mcpServers` object while preserving unrelated top-level
  configuration
* replaces AOS-managed Codex server tables while preserving unrelated tables
* expands `${HOME}` in endpoints, headers, commands, arguments, environments,
  and working directories
* maps `x-codex.defaultToolsApprovalMode` to Codex
  `default_tools_approval_mode`
* maps `x-codex.envVars` to Codex `env_vars`, the variables Codex forwards
  to a stdio server from its filtered environment, and
  `x-codex.toolTimeoutSec` to `tool_timeout_sec`
* maps `x-codex.envHttpHeaders` (header to variable name) to Codex
  `env_http_headers` and drops those headers from `http_headers`, because
  Claude expands a `${VAR:-default}` header value itself and Codex would send
  it literally. Codex skips a header whose variable is unset or blank

Supported Codex approval modes are `auto`, `prompt`, `writes`, and `approve`.
An omitted mode leaves the default in force. The first projection absorbs the
legacy Agent Compose block. Set `project_native: false` for server-class hosts
that need only the mcporter fallback.

## Apply and check

`aos converge` writes changed files atomically. `aos converge --check` performs
no network access and no filesystem mutation, and exits nonzero on catalogue,
manifest, or MCP drift.

## Ownership

AOS owns source selection, cache policy, host paths, and MCP and Codex-approval
projection. Infrastructure renders host config and invokes apply or check, and
Agent Compose receives the local manifest and owns role composition.

## AOS catalogue cache and manifest

The `catalogues` section of
[AOS environment convergence](aos-convergence.md) turns remote Git sources
into verified local roots. Agent Compose consumes the result without network
access or cache ownership.

## Locators

Each source is a scalar locator selecting a repository, optional path, and
revision:

```text
owner/repo/path@ref
https://example.test/owner/repo.git/path@ref
ssh://git@example.test/owner/repo.git/path@ref
```

A bare `owner/repo` uses GitHub. Locators without a path use
`.agents/skills`. Embedded HTTP credentials are rejected. Authentication
remains Git's concern and is never copied into the manifest.

## Cache behavior

AOS keeps one locked mirror and detached worktree per normalized source under
`state_dir/cache/catalogues`. The default freshness window is ten minutes.
Mutable stale refs refresh with `git remote update --prune`. A full commit SHA
already present in the mirror needs no network refresh.

First-use failures stop convergence. When a stale refresh fails, AOS reuses
the last checkout only when that checkout and catalogue directory remain
valid. The command reports this offline fallback as a warning.

`aos converge --check` never fetches or writes. A stale mutable ref, missing
checkout, or mismatched commit reports drift. An available immutable commit
remains current regardless of cache age.

## Manifest

The emitted JSON uses the stable `aos.catalogues.v1` format: a `catalogues`
array whose entries carry `source`, `path`, and `commit`. Entries preserve
declaration order, because downstream precedence can depend on it, so consumers
validate `format`, keep array order, and read `path` as the catalogue root.
`source` and `commit` make the input auditable without exposing credentials,
and the manifest is written atomically only after every configured source
resolves.
