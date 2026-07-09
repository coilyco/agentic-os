# dev-base auto-bump

The dev-base image pins every tool as a hand-edited `ARG`
([docs/dev-base-image.md](dev-base-image.md)). `release.yml` republishes the
image on every push to main, but nothing kept those pins current, so the
published image silently drifted behind upstream until a human edited an `ARG`.
That is the ward#288/#301 incident (agentic-os#272): headless agents kept hitting
a bug already fixed upstream because the image they ran predated the fix.

## What runs

[`.forgejo/workflows/dep-bump.yml`](../.forgejo/workflows/dep-bump.yml) runs
daily (and on `workflow_dispatch`). It calls
[`scripts/dep-bump.py`](../scripts/dep-bump.py), which resolves each pinned tool
against its upstream latest release, and for every pin that drifted commits a
single `ARG` bump and pushes to main. That push republishes the image through the
same `publish-image` job as any other push to main, so `:latest` tracks upstream.

The bump logic lives in this repo because the publish pipeline does (AGENTS.md).
Fleet rollout is not its concern.

## Properties

- **Auditable, not silent.** One commit per tool
  (`chore(dev-base): bump UV_VERSION 0.11.21 -> 0.11.24`), so history shows every
  bump and a bad one bisects to its own commit.
- **Conservative bumps.** Node and the .NET SDK track the latest release of their
  currently-pinned major (no surprise major jump - the SDK stays on the .NET 10
  channel eco-app's mods build against, agentic-os#329); uv, go, aws-cli, gh,
  helm, kubectl, yq, claude, mcporter, codex, goose, docker, tailscale,
  trufflehog, and ward track the latest stable upstream release.
  Tailscale resolves against its `pkgs.tailscale.com/stable` feed (not GitHub
  tags, which interleave the unstable odd-minor releases); kubectl resolves
  against Kubernetes' stable feed; ward resolves against its Forgejo tags list
  (it lives on Forgejo, not GitHub, and the image builds it from source at that
  pinned tag - agentic-os#223). A hand-edited `ARG` wins until upstream passes
  it.
- **Two deliberate opt-outs.** `GOLANGCI_LINT_VERSION` and `KDLFMT_VERSION` have
  no resolver, so the auto-bump never touches them (agentic-os#292). Their lint /
  format output is version-sensitive, and the in-container gate has to match the
  version the consuming repos' CI pins (cli-guard + ward pin golangci-lint
  v2.12.2; ward pins kdlfmt v0.1.7). Auto-bumping them to upstream latest would
  re-drift the container gate from CI - the exact failure the bake-in closed.
  Bump them by hand when the consumers move.
- **Job-local by design.** Docker buildx stays in the release workflow, and the
  Rust + wasm-pack toolchain stays in galaxy-gen. They are too workflow-specific
  to join the shared fleet baseline.
- **Fail-soft.** A resolver whose upstream is unreachable or has reshaped its API
  drops from that run with a warning; it never blocks the other bumps.

## Running it locally

- `ward exec dep-bump -- check` - drift summary, exit 1 if any pin is stale.
- `ward exec dep-bump -- plan` - the stale set as TSV (`--json` for machines).
- `ward exec dep-bump -- apply --arg UV_VERSION --version X` - rewrite one `ARG`.

## Republish token

The workflow pushes with the auto-issued job token by default, which is enough on
a Forgejo that enqueues workflow runs from bot-token pushes. If your Forgejo
suppresses them (the guard GitHub applies to prevent recursion), the bump lands
but `publish-image` never fires. Set the `DEP_BUMP_TOKEN` Actions secret to a
`coilyco-ops`-owned `write:repository` PAT so the push is attributed to a real
user and enqueues `release.yml`;
[`scripts/rotate-dep-bump-token.sh`](../scripts/rotate-dep-bump-token.sh) mints
and sets it (mirrors the `REGISTRY_TOKEN` rotation in dev-base-image.md).

## See also

- [docs/dev-base-image.md](dev-base-image.md) - the image and its publish path.
- [docs/release.md](release.md) - the release pipeline the republish rides on.
