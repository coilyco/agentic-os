---
name: tooling-mcp-servers
description: Shared native MCP inventory with mcporter discovery and CLI fallback. Hard-trigger before any curl, gh, or HTTP fallback.
---

# mcp-servers

## Triggers

mcp, mcporter.

The shared MCP layer. Configured servers live in `<personal-os-repo>/config/mcporter.json`. Host and container convergence copy that inventory to `~/.mcporter/mcporter.json` and project the same server set into every supported harness's native user registry. Typed headers per server live in `<personal-os-repo>/mcp-servers/*.d.ts`.

The point of this layout is one inventory and one projection contract. Claude Code and Codex both receive native registrations. `mcporter list` remains the flat discovery surface and `mcporter call` remains the portable fallback when a native tool is unavailable. Schemas stay deferred by each harness or by the explicit `.d.ts` read. See [Design notes](references/design-notes.md).

## Hard-trigger rule

If the agent is about to reach for **anything** that smells like an MCP-shaped capability (search a corpus, drive a browser, query an observability backend, hit a service over HTTP that has an MCP wrapper, etc.), this skill fires first. Falling back to `curl`, `gh`, raw HTTP, or "I'll just shell out" without checking the mcporter inventory is the bug. The inventory (or `mcporter list` when the inventory drifts) enumerates everything flat - read it before improvising.

## Sections

- [Inventorying and discovering servers](references/inventory-and-discovery.md) - inventory entry format plus `mcporter list` discovery commands and `.d.ts` regeneration.
- [Calling workflow and adding servers](references/workflow.md) - pick / read `.d.ts` / `mcporter call` / output flags, plus the add-a-server checklist.
- [Design notes](references/design-notes.md) - cross-cwd resolution, why no `.mcp.json`, and mobile / cloud MCP separation.
