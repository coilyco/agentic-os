# Inventorying and discovering servers

## Inventorying servers

This skill documents the **shape** of the shared native-MCP pattern. The actual server list belongs in the personal-OS repo and changes per user. Each entry in the index should be one line: name / category / what-it-does / auth-status / `Read <path>.d.ts`. Example:

```
* terraform / registry / latest provider + module versions, capabilities, details, search modules. No auth. Read mcp-servers/terraform.d.ts.
* playwright / browser / navigate, click, fill, screenshot, network logs, evaluate JS. Local stdio via npx @playwright/mcp. Read mcp-servers/playwright.d.ts.
* phoenix / LLM observability / Phoenix traces, spans, prompts, datasets, experiments. Local stdio. Read mcp-servers/phoenix.d.ts.
* sentry / observability / issues, events, projects, orgs. OAuth completed. Read mcp-servers/sentry.d.ts.
```

## Discovery commands (when the inventory drifts)

* `mcporter list` - all servers + tool counts + health (HTTP / auth / offline).
* `mcporter list <server>` - TypeScript-style signatures for that server's tools, inline.
* `mcporter list <server> --schema` - full JSON schema dump.
* `mcporter list --json` - structured per-server status for scripts.

When the inventory drifts from `mcporter list`, regenerate the `.d.ts` files:

```bash
cd <workspace-root>
for srv in $(mcporter list --json | jq -r '.servers[].name'); do
  mcporter emit-ts $srv --out <personal-os-repo>/mcp-servers/$srv.d.ts --mode types
done
```

Then update the inventory list.
