# Warded-agent git identity

Per the self-name display stamp documented in
[docs/dev-base-self-name.md](dev-base-self-name.md), a warded aos session keeps
its self-name on the status line and the session banner. Git identity is
separate: the committer NAME and EMAIL resolve to the deployment bot so Forgejo
links the commit to the coilyco-ops account instead of an example fallback.

## What runs

The image bakes the bot identity into git's system config in
[`docker/dev-base/agent/Dockerfile`](../docker/dev-base/agent/Dockerfile), so
every user inherits `coilyco-ops <coilyco-ops@coilysiren.me>` without a runtime
write. The baked [`agent-name.sh`](../docker/dev-base/agent-name.sh) still
carries a `gitidentity` mode as a fallback for older or custom images, and the
policy-tier [`managed-settings.json`](../docker/dev-base/claude-managed-settings.json)
wires it as a second `SessionStart` hook, right after the self-name banner.

## Why SessionStart, not the ward entrypoint

SessionStart is the earliest the self-name banner and bot identity hook can run
with the full environment in place. The distinguishing `<tag>` is derived from
the `session_id`, which the ward entrypoint cannot know before the agent starts.
The hook now backfills only when the image or runtime environment lacks the
baked identity, so it respects the Dockerfile-owned config instead of
overwriting it.

## Why the bot identity is explicit

The committer identity must not drift back to ward's example bot defaults. The
image-level config now owns the default, with `WARD_GIT_NAME` and
`WARD_GIT_EMAIL` as override knobs for custom image builds or fallback writes.

## Scope and limits

- **Best-effort.** A git failure is swallowed so it never breaks session start.
- **claude only today.** The stamp rides claude's policy-tier settings; other
  harnesses still need their own hook path if they want the same bot identity.
  A cross-harness stamp would belong in ward's entrypoint, where every harness
  passes through.
- **Dockerfile owns the baseline.** The runtime hook is fallback only.

## See also

- [docs/dev-base-self-name.md](dev-base-self-name.md) - the self-name this builds on.
- [docs/dev-base-image.md](dev-base-image.md) - the image both ride in.
- [docs/FEATURES.md](FEATURES.md) - feature inventory.
