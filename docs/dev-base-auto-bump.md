# dev-base auto-bump

The dev-base Dockerfiles pin every tool as a hand-edited `ARG`
([docs/dev-base-image.md](dev-base-image.md)). `promote.yml` republishes the
image on every push to main before `release` moves, but nothing kept those pins
current, so the image drifted behind upstream until a human edited an `ARG`.
That is the ward#288/#301 incident (agentic-os#272): headless agents kept
hitting a bug already fixed upstream because the image they ran predated the
fix.

## What runs

[`.forgejo/workflows/dep-bump.yml`](../.forgejo/workflows/dep-bump.yml) runs
daily (and on `workflow_dispatch`). It calls
[`scripts/dep-bump.py`](../scripts/dep-bump.py), which resolves each pinned tool
against its upstream latest release, and for every pin that drifted commits a
single `ARG` bump and pushes to main. That push republishes the image through
the same `publish-dev-base` job as any landed commit, so the moving `:release`
alias tracks upstream.

The bump logic lives in this repo because the publish pipeline does (AGENTS.md).
Fleet rollout is not its concern.

After the planned bumps are applied, the workflow runs the same gate as release
before it pushes, so a bad pin leaves the run red and stops before `main`.

## Properties

- **Auditable, not silent.** One commit per tool
  (`chore(dev-base): bump UV_VERSION 0.11.21 -> 0.11.24`), so history shows every
  bump and a bad one bisects to its own commit.
- **Conservative bumps.** Node and the .NET SDK track the latest release of their
  currently-pinned major (no surprise major jump - the SDK stays on .NET 10,
  agentic-os#329); uv, go, aws-cli, claude, mcporter, codex, goose, gh, helm,
  kubectl, yq, docker, tailscale, and trufflehog track latest stable.
  Tailscale resolves against its `pkgs.tailscale.com/stable` feed (not GitHub
  tags, which interleave the unstable odd-minor releases). A hand-edited `ARG`
  wins until upstream passes it.
- **Three deliberate opt-outs.** `GOLANGCI_LINT_VERSION`, `KDLFMT_VERSION`, and
  `WARD_VERSION` have no resolver, so auto-bump never touches them
  (agentic-os#292). Lint/format pins must match consumer CI. Ward is manual while
  raw releases are staging: aos should advance prod/N-1 only after validating
  candidate N against the real `.ward` bundle. `docker buildx` and `wasm-pack`
  stay job-local, so dev-base does not claim them.
- **Fail-soft.** A resolver whose upstream is unreachable or has reshaped its API
  drops from that run with a warning. It never blocks the other bumps.

## Running it locally

- `ward exec dep-bump -- check` - drift summary, exit 1 if any pin is stale.
- `ward exec dep-bump -- plan` - the stale set as TSV (`--json` for machines).
- `ward exec dep-bump -- apply --arg UV_VERSION --version X` - rewrite one `ARG`.

## Republish token

The workflow pushes with the auto-issued job token by default, which is enough on
a Forgejo that enqueues workflow runs from bot-token pushes. If your Forgejo
suppresses them, the bump lands but the release promotion never fires. Set the
`DEP_BUMP_TOKEN` Actions secret to a `coilyco-ops`-owned `write:repository`
PAT so the push is attributed to a real user and reliably reaches the
promotion flow;
[`scripts/rotate-dep-bump-token.sh`](../scripts/rotate-dep-bump-token.sh) mints
and sets it (mirrors the `REGISTRY_TOKEN` rotation in dev-base-image.md).

## See also

- [docs/dev-base-image.md](dev-base-image.md) - the image and its publish path.
- [docs/release.md](release.md) - the release pipeline the republish rides on.
