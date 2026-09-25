# Architecture

The client is one Svelte app with no router. State lives in `src/lib/app.svelte.ts`, and the sidebar's tabs choose what the main pane shows.

## The host connection

`HostConnection` in `src/lib/protocol.ts` is the whole seam: subscribe, attach, detach, type, resize, launch. `DaemonHost` maps it onto `aterm.daemon.v1` over the daemon's loopback websocket (agentic-os `docs/aterm-daemon.md`, Wire contract), and `MockHost` scripts the same events for the Demo host. The terminal subscribes before it attaches, so the replay an attach triggers is never missed. Launching sends the daemon's `launch` frame, which runs `aterm <role> [seat]` on the host and opens a real window there. The launch state lives per role until the session appears on the sessions channel.

## Why envelopes are matched, not reported

A peer message is typed into the target harness's input, and the harness decides where it echoes it, often behind its own prompt. The daemon cannot know which screen rows hold it, and its message events carry no body. The daemon does escape envelope-shaped lines inside a body with a leading backslash, so the client marks a logical line holding an unescaped `[from <role> <identity>]` from a sender it has a message event for. Wrapped rows are joined first, so a narrow pane still marks the whole line. A seat quoting another's envelope in its own output would also be marked, which is cosmetic.

## Narrow screens

Below 720px the sidebar becomes two horizontal tab strips, and below 1000px the messages panel moves under the terminal.
