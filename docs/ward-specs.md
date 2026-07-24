# Ward Spec Bundle

The coilyco ward bundle lives in [`.ward/`](../.ward/) beside `ward.yaml`. It
carries role-scoped guarded sources, Actions bridges, agent/role/repo policy, and
[`defaults.kdl`](../.ward/defaults.kdl), which selects the aos agent image and
`release` tag. Aos tracks no upstream Forgejo OpenAPI blob.

## Direction of truth

**aos is the source of truth for the coilyco ward-specs bundle.** Since the
ward#503 producer cutover (2026-07-07), coilyco deployment values are authored
here and flow down into ward at release time, not the reverse. This replaces
the older shape where ward held the canonical values and aos mirrored them.
When a coilyco fleet, guardfile, role-catalog, spec-lock, or launch-default
value changes, change it here in aos's `.ward/` and let a push republish the
bundle. The launch defaults include the aos agent image on its moving
`release` tag and fleet `merge-remote-main`, with cli-guard, ward, and
agentic-os on `pull-request-and-merge` (canonical ward#508 spellings).

This is the one place a shipped tool (ward) consumes runtime config authored in
a reference repo (aos), a deliberate exception to AGENTS.md's config-placement
corollary. The bundle is Kai's single coilyco deployment, not fleet config
every ward user melds. Forgejo is split into role-facing read, write, admin,
merge, Actions-read, and runner-token guardfiles. The operator monolith moved
to [Aguard](aguard.md). Ward keeps only the dynamically selected role policy.
The exception is stated in [AGENTS.md](../AGENTS.md).

See [role overlays](ward-specs-overrides.md) and
[local model ownership](ward-local-models.md).

## Profile asset home

When Ward consumes typed profile data, AOS owns the surviving profile and
config assets under [ward-profile-assets.md](ward-profile-assets.md).

## Release asset

Every aos release attaches the bundle as `ward-specs-<tag>.tar.gz` (plus a
`.sha256` sidecar) via the release publication job in
[`.forgejo/workflows/promote.yml`](../.forgejo/workflows/promote.yml), with
[`.forgejo/workflows/release.yml`](../.forgejo/workflows/release.yml) kept as a
manual retry path. The
tarball is deterministic, so the checksum a downstream pins is reproducible from
the tag. Ward's build sites pin that URL + `sha256` rather than a raw tracked
path, and the packaging step walks `.ward/` recursively while excluding
`.ward/ward.yaml`, so new bundle files land automatically and the overlay input
stays clean.

## How ward consumes it

ward keeps one neutral shipped binary and selects the coilyco bundle at launch
through `WARD_CONFIG_REF` for the guarded edge surfaces. The published
`ward-specs-<tag>.tar.gz` remains the canonical bundle artifact and checksum
target, but the live config path is the runtime `WARD_CONFIG_REF` seam, not a
bespoke rebuild from the asset. Ward's embedded image stays neutral, while
[`defaults.kdl`](../.ward/defaults.kdl) selects the aos image and tag.

The bundle uses source binary names that Ward reroots to the selected role
command at runtime. Role-only Forgejo tier files use `aos-agent`. Operational
automation and human operators use `aguard ops ...` instead of loading the Ward
bundle directly.

Landing policy lives in [`.ward/repos.kdl`](../.ward/repos.kdl). Its workflow
block keeps the coilyco PR-gated repos explicit.

Host shells and the container entrypoint point `WARD_CONFIG_REF` at the
checkout's `.ward/` live (`file://`): no commit pin to rot in a long-lived
terminal, no gitsync or git credential at launch (the stale-pin fail-closed
chain behind aos#452/aos#472). Only the dev-base image build bakes a
commit-pinned ref, the fallback without a seeded checkout.
