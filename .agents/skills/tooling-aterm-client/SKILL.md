---
name: tooling-aterm-client
description: Work on the aterm web client in aterm-client/ (Svelte 5, TypeScript, Vite, xterm.js), the window onto aterm agent sessions. Covers its architecture, native choice cards, views and browser, how it is built and served, and the rules for changing it. Triggers - aterm client, aterm-client, svelte, xterm, aterm.daemon.v1 client, waiting seats, glow, toast.
---

# aterm client

The client surface for aterm sessions: layout, components, terminal rendering, and
the words on screen. It lives in `aterm-client/`, beside the daemon in `aterm/`
that owns the protocol and the PTYs.

## Shape

Svelte 5, TypeScript, and Vite, with xterm.js for terminals. `src/lib/` holds the
state, the roster decoder, and the mock host. `src/components/` and `src/panels/`
hold the surface. The design reference is the aterm host app flows canvas linked
from `teable:coilyco/agentic-os#8220`.

## Boundaries

`aterm.daemon.v1` is owned by the daemon in `aterm/` ([aterm-daemon.md](../../../docs/aterm-daemon.md)).
`src/lib/daemon-host.ts` follows it and never extends it. The mock scripts only
behaviour the daemon has, so the Demo host never promises what "this Mac" cannot do.

Deployment config does not live here: hosts, allowed origins, tags, which roles
appear, and publishing belong to the deployment, which passes them in at build
time. Nothing here fetches it at runtime.

## Commands

Every command is a root `just` verb: `aterm-client-install`, `-dev`, `-check`,
`-test`, `-build`, `-gate` (the CI gate), `-embed` (stage the build for the aterm binary), `-install-dir`, `-build-hosted`, and
`-sync-creatures`. [running.md](references/running.md) says what each does.

## Validation

Run `just aterm-client-gate` and `pre-commit run --all-files` before committing. A
layout change is not done until it has been rendered at 320px wide and walked by
keyboard.

## Contracts

The roster fixture in `src/lib/fixtures/roster.json` is a snapshot of
`aterm --list --json` (`aterm.roster.v1`). Refresh it from the live command rather
than editing it by hand.

## References

* [features.md](references/features.md) - what ships today.
* [context-meter.md](references/context-meter.md) - the per-seat context-token count, where each harness's reading comes from, and Kai's freeze exception.
* [architecture.md](references/architecture.md) - the host seam, envelopes, activity, the composer.
* [discovery.md](references/discovery.md) - the `hosts` frame and how the host list merges the daemons it finds.
* [choices.md](references/choices.md) - native choice cards and `ask_choice`.
* [grants.md](references/grants.md) - approval grants: one Kai decision carried to the executing seats, and the daemon's refusals.
* [passkey.md](references/passkey.md) - the passkey unlock a remote device needs before it types.
* [reach.md](references/reach.md) - why a device cannot reach the daemon, the layers the client names, and what the daemon logs.
* [sizing.md](references/sizing.md) - how a session's PTY is sized when clients of different sizes attach, the `size` frame, and `pty-size`.
* [device-key.md](references/device-key.md) - the Android app's unlock with the phone's own key, as a person sees it.
* [device-key-daemon.md](references/device-key-daemon.md) - the daemon half: the frames, the attestation checks, and the app origin.
* [views-and-browser.md](references/views-and-browser.md) - MCP Apps views and the shared browser.
* [browser.md](references/browser.md) - the daemon's streamed Chromium and its `browser_*` frames.
* [web-push.md](references/web-push.md) - the daemon's Web Push to a closed browser, the frames, and what the service worker builds against.
* [terminal-split.md](references/terminal-split.md) - the Terminal tab, the split, and the side panel's default tab by role.
* [mcp-apps-gateway.md](references/mcp-apps-gateway.md) - the daemon gateway and the `views` frames a client receives.
* [deploy.md](references/deploy.md) - served by a daemon, or hosted.
* [running.md](references/running.md) - alt-tab use and running it.
* [slack-adapter.md](references/slack-adapter.md) - the Slack client design. Kai froze new work on this web client on 2026-10-02 (`teable:coilyco/agentic-os#8700`), so merged code stays and features go to Slack.
