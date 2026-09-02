---
name: tooling-claude-in-chrome
description: Drive live Chrome via the Claude_in_Chrome MCP for logged-in web automation. Use JS over mouse (mouse trips the chrome-extension boundary). Never leave forms dirty (Leave site? modal wedges it).
---

# claude-in-chrome

## Triggers

claude in chrome, chrome mcp, browser automation, remote browser, drive chrome, browser_batch, javascript_tool, scrape page, Codex environments, "different extension" error, Leave site dialog.

The `mcp__Claude_in_Chrome__*` MCP attaches to a live local Chrome via the Claude in Chrome extension. Real auth, real cookies, real session state. Use it when a target has no usable API and clicking through is the only option (Codex env creation, OpenAI settings, recruiter portals, vendor dashboards).

## When to reach for it

* The site has no public/working API for the action.
* The action depends on a logged-in browser session that lives only in the human's Chrome.
* The repetition count is high enough that a checklist isn't faster (rough rule: 8+ identical clicks).

If fewer than ~5 clicks, just hand the human a checklist with deep links. Browser automation has setup cost.

## Two rules that keep the session alive

- [keep the session alive](references/keep-session-alive.md) - JS over mouse, and never leave a form dirty (the two non-negotiables).

## Patterns

- [React input and per-row flow](references/react-input-and-flow.md) - native-setter pattern for React-controlled inputs, and the reliable navigate-search-click-submit-verify loop.
- [wedge recovery and gotchas](references/wedge-recovery-and-gotchas.md) - unwedging a dead session, browser_batch and dialog gotchas, and the anti-signals that mean "use JS, not mouse".
