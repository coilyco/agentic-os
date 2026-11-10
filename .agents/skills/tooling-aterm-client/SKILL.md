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
keyboard. `just aterm-client-drive` does that with no human: it drives the real client
against the Demo host in a headless browser at 1280px and 320px, on its own port, and
asserts what is on screen. Check a UI change there and never hand Kai a click to test.
Add a case when a bug appears, and teach the Demo host the screen that reproduces it
(`src/lib/demo-harness.ts`). `just aterm-client-drive-hops` does the same for a connection that fails: it breaks one hop per scenario in a mock daemon (`scripts/hop_daemon.py`) and checks the connection check names it.

## Contracts

The roster fixture in `src/lib/fixtures/roster.json` is a snapshot of
`aterm --list --json` (`aterm.roster.v1`). Refresh it from the live command rather
than editing it by hand.

## References

* [features.md](references/features.md) - what ships today.
* [testbed.md](references/testbed.md) - the test bed: journeys against a mock daemon at desktop and phone sizes, baselines, and reported quirks.
* [android-emulator.md](references/android-emulator.md) - `just aterm-android-emulator`: drive one live emulator running the real Treehouse app, the lock, the debug APK, and where it runs.
* [android-emulator-rig.md](references/android-emulator-rig.md) - starting and stopping the whole emulator rig, and the one-tap check for a choice card in the real app.
* [tap-trace.md](references/tap-trace.md) - the `?taptrace` flag: an on-device record of a tap or submit, one Copy button, no seat content.
* [context-meter.md](references/context-meter.md) - the per-seat context-token count, where each harness's reading comes from, and Kai's freeze exception.
* [architecture.md](references/architecture.md) - the host seam, envelopes, activity, the composer.
* [discovery.md](references/discovery.md) - the `hosts` frame and how the host list merges the daemons it finds.
* [feedback.md](references/feedback.md) - the Feedback sheet, and the daemon's `feedback` frames that keep its note and screenshots as a durable record handed to a frontend-eng seat.
* [choices.md](references/choices.md) - native choice cards, `ask_choice` and `send_user_question`.
* [seat-state.md](references/seat-state.md) - how long a seat has been in its state, and its last reply.
* [transcript.md](references/transcript.md) - the daemon's `transcript` read: a seat's prompts, replies and tool steps from its harness files, the frame, peer messages, and how each harness records a tool step.
* [session-names.md](references/session-names.md) - session refs, name@ref, and why a close by stale name is refused.
* [grants.md](references/grants.md) - approval grants: one Kai decision carried to the executing seats, and the daemon's refusals.
* [passkey.md](references/passkey.md) - the passkey unlock a remote device needs before it types.
* [tailnet-access.md](references/tailnet-access.md) - which tailnet devices attach, owner logins on a tagged node, and why a Tailscale Serve front is never loopback.
* [linux-seat.md](references/linux-seat.md) - what the listeners allow a seat user on Linux, and the probe that checks it (COI-3498).
* [reach.md](references/reach.md) - why a device cannot reach the daemon, the layers the client names, what the daemon logs, and the six-hop connection check with what a page can and cannot see.
* [media.md](references/media.md) - the viewer for images, video and files a seat shares, the sequence, the file fallback, and trying it with no daemon that shares files.
* [sizing.md](references/sizing.md) - how a session's PTY is sized when clients of different sizes attach, the `size` frame, and `pty-size`.
* [desktop.md](references/desktop.md) - the windowed Treehouse app for the Mac: its identity, what it does at launch, and the verbs that build and install it.
* [android-opener.md](references/android-opener.md) - why a window does nothing for window.open, and how an artifact or link opens in the device's own browser.
* [android-sharetarget.md](references/android-sharetarget.md) - Treehouse as an Android Share target for images: the sheet, the plugin, and what is not yet checked on a phone.
* [device-key.md](references/device-key.md) - the Android app's unlock with the phone's own key, as a person sees it.
* [device-key-daemon.md](references/device-key-daemon.md) - the daemon half: the frames, the attestation checks, and the app origin.
* [resume.md](references/resume.md) - the unlock that survives a phone lock: the resume proof, the window setting, the gate it leaves closed, and the Pixel procedure.
* [treehouse-broker.md](references/treehouse-broker.md) - the brokered path: the three settings, how a brokered channel unlocks with an accounts token, and what is not built yet.
* [views-and-browser.md](references/views-and-browser.md) - MCP Apps views and the shared browser.
* [browser.md](references/browser.md) - the daemon's streamed Chromium and its `browser_*` frames.
* [web-push.md](references/web-push.md) - the daemon's Web Push to a closed browser, the frames, and what the service worker builds against.
* [image-upload.md](references/image-upload.md) - the `image_upload` frame: a pasted image becomes a file on the host, where it lives, the cap, and the guards.
* [media-daemon.md](references/media-daemon.md) - `aterm show`, the `media` channel and `media_read`: the daemon half, and its guards.
* [open-url.md](references/open-url.md) - `aterm open`, the `open_url` frame, the shims every seat launches with, and the native fallback.
* [artifacts.md](references/artifacts.md) - `aterm artifact`, the `artifact` frames on the views channel, the path and size limits, and what a standing page does when its seat ends.
* [terminal-split.md](references/terminal-split.md) - the Terminal tab, the split, and the side panel's default tab by role.
* [mcp-apps-gateway.md](references/mcp-apps-gateway.md) - the daemon gateway and the `views` frames a client receives.
* [github-token.md](references/github-token.md) - `aterm github-token` and `aterm git-credential`: a seat pushes as the coilyco-ops App, granted wide and minted narrow.
* [deploy.md](references/deploy.md) - served by a daemon, or hosted.
* [running.md](references/running.md) - alt-tab use and running it.
* [slack-adapter.md](references/slack-adapter.md) - the Slack client design. Kai froze new work on this web client on 2026-10-02 (`teable:coilyco/agentic-os#8700`), so merged code stays and features go to Slack.
