# Design notes: cross-cwd, no-mcp-json, mobile

## Cross-cwd resolution

mcporter layers local config with imports from harness-native registries unless the inventory says otherwise. A canonical inventory should set top-level `"imports": []`, which makes it authoritative and prevents stale native-only entries from flowing back into `mcporter list`.

`agent-compose mcp --inventory <path>` copies the canonical source to `~/.mcporter/mcporter.json` and hard-projects its `mcpServers` into the supported harness registries. Bare `agent-compose` runs the same projection when `mcp_inventory` is configured. Host configuration and ephemeral-container bootstrap invoke this one contract rather than implementing per-harness sync scripts.

## One native projection

Claude Code and Codex receive the same native server inventory. The projector replaces only its owned MCP set and preserves unrelated harness configuration. Removing a server from the canonical inventory removes it from both native registries at the next convergence.

This is not a checked-in `.mcp.json`. User scope applies host-wide across every cwd and needs no per-project approval gate. `mcporter` remains the harness-neutral inventory, schema browser, and call fallback. Harness-native tools remain the preferred invocation path when present.

## Built-ins and connectors stay outside the inventory

The projector owns only `mcpServers` from the canonical inventory. Harness built-ins, plugins, and cloud connectors are separate surfaces and need their own inventory. Disable unwanted extras explicitly rather than adding placeholder entries to mcporter. Claude Code accepts `deniedMcpServers` entries such as `{"serverName": "claude-in-chrome"}` and has a separate `disableClaudeAiConnectors` switch. Codex built-in MCPs can be kept outside the managed block with `enabled = false`.

## Mobile / cloud MCP

mcporter is a local CLI for Mac and Linux. Anthropic cloud connectors used by mobile Claude or claude.ai are a separate channel and are unaffected. Mobile sessions still see whatever MCPs are wired on the cloud side. This skill covers local harness registries on hosts and in ephemeral agent containers.
