# Tiered dev-base image split

This is the implemented tier split for dev-base. The old monolithic Dockerfile
became one folder per published tier, with `dev-base-full` the default surface.

## Tier layout

- `docker/dev-base/core/Dockerfile` - Ubuntu, certs, shell/git, `python3`, `uv`, `pre-commit`, `ward`, Rust, and the hidden `dev-base-ward-builder` stage that compiles `ward`.
- `docker/dev-base/lang-node/Dockerfile` - Node and npm.
- `docker/dev-base/lang-go/Dockerfile` - Go for repos that build or test Go in CI.
- `docker/dev-base/lang-dotnet/Dockerfile` - .NET SDK and ICU.
- `docker/dev-base/ops/Dockerfile` - `aws`, `gh`, `helm`, `kubectl`, `yq`, Docker client, Tailscale client, and `tailscaled`.
- `docker/dev-base/agent/Dockerfile` - Claude, Codex, Goose, mcporter, self-name assets, substrate seed.
- `docker/dev-base/full/Dockerfile` - fan-in image for general `warded` use and the default surface.

## Dependency graph

The tiers form a fan-out/fan-in DAG (aos#491):

- `core` is the root; `lang-node`, `lang-go`, `lang-dotnet`, and `ops` are
  siblings on it, each carrying only its own toolchain.
- `agent` builds from `ops` and **grafts** Node in (Claude Code rides Node).
  `full` builds last from `agent` and grafts `lang-go` and `lang-dotnet`.
- Docker images have one parent, so composed tiers graft sibling toolchains
  with `COPY --from=` of self-contained prefixes (`/usr/local/node`, `/go`,
  `/dotnet`), all on `core`'s `PATH`. `TierSpec.base_tier` / `graft_tiers` in
  [`agentic_os/dev_base.py`](../agentic_os/dev_base.py) emit the
  `BASE_IMAGE` / `<TIER>_IMAGE` build-args.
- The hidden builder stage stays inside `core` so ward still compiles per target platform.

## Tag derivation

- One repo release tag drives the whole family.
- Everything publishes under the single `agentic-os` package, with the tier in the tag: `core` becomes `agentic-os:core-${TAG}` while `full`, the default surface, keeps the plain `agentic-os:${TAG}`.
- The release helper in [`scripts/dev-base-build.py`](../scripts/dev-base-build.py) derives the plan from the directory layout.
- Every published ref derives from `{registry base, folder name, tag}` - there is no checked-in manifest JSON to drift out of step with the folder layout.

## Release flow

- `release.yml` computes the next tag first, then runs **one publish job per
  tier**
  ([`actions/publish-dev-base-tier`](../actions/publish-dev-base-tier/action.yml)),
  with `needs:` carrying the DAG above: siblings build in parallel, a flaky
  tier fails and reruns alone, and a `lang-dotnet` flake no longer takes
  `ops` or `agent` down - only `full` waits on it.
- Each job verifies its pushed tag and alias manifests; the tag-cutting
  `release` job needs `publish-full`, so the tag lands only after the
  whole family. Builder and layer cache persist between runs:
  [dev-base build cache](dev-base-build-cache.md).
- Every pushed tier carries a moving alias named for the publishing branch (`:release` in the two-stage flow) alongside the release tag, so `dev-base-full` still fans in last and keeps the default `agentic-os:release` surface ward pulls. The full image also republishes `agentic-os-full:latest` for older launches.

## ARG ownership

- `UV_VERSION` and `WARD_VERSION` live in `core`.
- `NODE_VERSION` lives in `lang-node`.
- `GO_VERSION` lives in `lang-go`.
- `DOTNET_VERSION` lives in `lang-dotnet`.
- `AWSCLI_VERSION`, `GH_VERSION`, `DOCKER_VERSION`, `HELM_VERSION`, `KUBECTL_VERSION`, `YQ_VERSION`, and `TAILSCALE_VERSION` live in `ops`.
- `CLAUDE_VERSION`, `MCPORTER_VERSION`, `OPENCODE_VERSION`, `CODEX_VERSION`, and `GOOSE_VERSION` live in `agent`.
- `GOLANGCI_LINT_VERSION`, `TRUFFLEHOG_VERSION`, and `KDLFMT_VERSION` live in `full`.

## See also

- [dev-base container image](dev-base-image.md) - the current published contract.
- [CI parity in dev-base](ci-in-dev-base.md) - the pinned consumer side.
- [FEATURES.md](FEATURES.md) - the feature inventory this lands in.
