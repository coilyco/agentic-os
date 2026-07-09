# dev-base container image

aos owns the agent dev environment as a published artifact - the analog of the
ward brew binary, pulled not built. ward consumes it by tag and never touches the
Dockerfile, so config cannot drift. Part of the dockerized-dev epic
(see docs/features-release-tooling.md for the release-tooling background).

## What ships

[`docker/dev-base/Dockerfile`](../docker/dev-base/Dockerfile) layers the
inner-loop toolchain on `ubuntu:24.04`:

- **uv + managed Pythons** - Python project/tool manager; 3.13 + 3.12 pre-installed under a world-writable `UV_PYTHON_INSTALL_DIR` so a non-root agent never hits `Permission denied` on `uv run` (agentic-os#327).
- **pre-commit** - the catalog hook driver (a uv tool).
- **python3 + shellcheck + git + build-essential** - direct needs and catalog-hook shell-outs.
- **node + npm** - Claude Code's runtime.
- **go** - builds the `warp/` hooks and the ward binary below.
- **gh + helm + kubectl + yq** - shared fleet CI utilities that were getting
  reinstalled in consumer workflows and are light enough to bake into the image.
- **.NET SDK 10 + ICU** - C# mods compile in-container with no per-run install, full ICU globalization (`libicu74`) not invariant mode (agentic-os#329).
- **aws cli v2** - SSM secret loader + `~/.aws` passthrough; `AWS_DEFAULT_REGION` / `AWS_REGION` default `us-east-1` (agentic-os#286).
- **claude + mcporter + codex + goose** - pinned agent CLIs and MCP runtime; plus **docker cli + socat** for `explore`'s sibling `warded #N` dispatch (ward#315).
- **ward** - the dev-command surface agents route through (`ward <verb>`), built from source at the pinned `WARD_VERSION` tag, baked in not `go install`-ed per run - public source clones with **no build token** (agentic-os#223).
- **golangci-lint + trufflehog + kdlfmt** - lint / secret-scan / format binaries the gate shells out to, self-run in-container (agentic-os#292).
- **tailscale cli** - tailnet client (no daemon) so a credentialed container reaches the tower; auth stays ward's axis (agentic-os#286).
- **public substrate seed** - bare mirrors of the image-tier reference repos at `/opt/substrate-seed`, so a cold gitcache hydrates offline.
- **in-container agent self-name** - baked `agent-name.sh` + policy `managed-settings.json` so warded agents self-name ([doc](dev-base-self-name.md)).

Every tool installs world-readable under `/usr/local` or `/opt`, so the image runs
as any uid. ward owns the run-as-uid, mounts, and `~/.aws` passthrough; it bakes in
no user and no target repo.

## Naming and tags

Published to the forgejo registry as
`forgejo.coilysiren.me/coilyco-flight-deck/agentic-os`. Each release tags `vX.Y.Z`
and moves `:latest`, so a pin and its image share a version; `:buildcache` holds
the layer cache.

## How it publishes

The `publish-image` job in [`.forgejo/workflows/release.yml`](../.forgejo/workflows/release.yml)
runs after a release cuts a tag, on the DinD `docker` runner: `docker login` with
the `REGISTRY_TOKEN` secret, then `buildx build --platform linux/amd64,linux/arm64
... --push` with a registry layer cache, tagging `:vX.Y.Z` and `:latest`, so each
arch pulls a native image off the cache. `REGISTRY_TOKEN` is a `coilyco-ops`
`write:package` PAT ([`rotate-registry-token.sh`](../scripts/rotate-registry-token.sh)
re-mints it); the ward build clones public source anonymously, no secret here.

## Pinning a tool

Versions pin as `ARG`s: hand-edit and push to main to pin or roll back, else a
scheduled **auto-bump** refreshes stale pins ([auto-bump doc](dev-base-auto-bump.md)).
`GOLANGCI_LINT_VERSION` and `KDLFMT_VERSION` opt out, bumped by hand.

## Pulling it

```bash
docker pull forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:latest
```

Needs a one-time `docker login`; `ward container up/exec` (ward#98) is the entry point.

## Not here

- Mount / compose logic and `ward container` verbs - ward#98; the mount-eligibility manifest - aos#222.
- Docker buildx - release-image publishing keeps its local runner-side install.
- Rust + wasm-pack - galaxy-gen keeps that toolchain job-local.
- `coily` (retired, folded into `ward ops`) and running services - not shipped.

## See also

- [release.md](release.md) - the pipeline this rides on; [FEATURES.md](FEATURES.md).
