# Architecture

The client is one Svelte app with no router. State lives in `src/lib/app.svelte.ts`, and the sidebar's tabs choose what the main pane shows.

## The host connection

`HostConnection` in `src/lib/protocol.ts` is the whole seam to the daemon: subscribe to events, send input, launch a seat. `MockHost` implements it with scripted sessions so the surface can be built before the daemon exists (`teable:coilyco/agentic-os#8219`). Swapping in the websocket client changes one constructor in `selectHost`.

## Why envelopes are matched, not reported

A peer message is typed into the target harness's input, and the harness decides where it echoes. The daemon cannot know which screen rows hold it. The daemon does escape envelope-shaped lines inside a body, so an unescaped `[from <role> <identity>]` at the start of a logical line can only be one it stamped. The terminal scans its buffer for those lines and draws the stripe, joining wrapped rows first so a narrow pane still marks the whole message.

## Narrow screens

Below 720px the sidebar becomes two horizontal tab strips, and below 1000px the messages panel moves under the terminal.
