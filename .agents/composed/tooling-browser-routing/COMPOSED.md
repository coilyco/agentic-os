---
name: tooling-browser-routing
description: Route browser work across in-app and Playwright surfaces. Triggers - browser automation, UI testing, screenshots, no browser available, browser not installed, browser disconnected, transport closed.
---

# Browser automation routing

Choose the browser surface before making a connection attempt. A transport is
not a browser by itself, and a failed transport should not consume repeated
retries while a purpose-built Playwright surface is available.

## Selection order

1. When the user explicitly names the in-app Browser, Chrome, Edge, or an
   existing signed-in browser, use that exact surface and follow its skill. Do
   not substitute another browser without the user's approval.
2. For generic coding-agent browser work, use Playwright CLI only when the
   harness already exposes its installed skill and a governed command surface.
   Do not install or bootstrap it ad hoc during an unrelated task.
3. Otherwise use `playwright_local`. It is the default for localhost, local
   development, screenshots, page inspection, and isolated browser work.
4. Use `playwright_k3s` when the task needs its durable shared browser context,
   or as the fallback for a non-local target when `playwright_local` is
   unavailable. Its callers share tabs, cookies, and page state, so never treat
   it as isolated.

## Recovery

The Node-backed JavaScript runtime is only the transport for the in-app Browser
or browser-extension plugin. Do not use it to import standalone Playwright or
to improvise a browser.

When the user did not explicitly select that browser and its runtime reports no
available browsers, perform the plugin's required single discovery or
troubleshooting check, then stop retrying and continue with the selection order
above. Do not reset the runtime, repeat browser discovery, or abandon the task
before trying an available Playwright surface.

If `playwright_local` fails once, use `playwright_k3s` only when the target is
reachable from the hosted environment. A hosted browser cannot stand in for a
service bound to the calling host's localhost. If the hosted surface fails and
the target is reachable locally, return to `playwright_local` once. Report the
concrete blocker after those bounded attempts.

## CLI admission

Playwright's upstream guidance favors CLI and skills for token-efficient
coding-agent workflows, while retaining MCP for persistent exploratory loops.
That efficiency claim does not prove local reliability. Admit CLI as the
generic default only after its package, browser binaries, skill projection,
command rules, and representative success and recovery paths are installed
and verified on the execution surfaces that will use it.
