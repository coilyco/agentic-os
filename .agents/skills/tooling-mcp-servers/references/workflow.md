# Calling workflow and adding servers

## Workflow

1. **Pick the server** from the personal inventory based on the task.
2. **Read the `.d.ts`** for that server. Each tool's signature, JSDoc, and required vs optional parameters are in there. Do not skip this step. The schema is what makes the call correct.
3. **Call the native tool** when the harness exposes it. Otherwise call through `mcporter` with `key=value` flags or function-call syntax:
   ```bash
   mcporter call terraform.get_latest_provider_version name=aws namespace=hashicorp
   mcporter call 'phoenix.list-projects()'
   ```
   `mcporter <server>.<tool> ...` (no `call` verb) also works as shorthand.
4. **Output**: defaults to pretty text. Add `--output json` or `--raw` for machine-readable. `--json` on `list` for structured server status.

## Adding a new MCP server

1. Add the server entry to `<personal-os-repo>/config/mcporter.json`. Keep top-level `"imports": []` so the canonical inventory does not absorb stale harness-native entries.
2. If OAuth-protected: `mcporter auth <name>`.
3. `mcporter emit-ts <name> --out <personal-os-repo>/mcp-servers/<name>.d.ts --mode types`.
4. Add a one-line entry to the inventory.
5. Re-run the personal-OS repo's host convergence. Its `agent-compose mcp` call updates the home inventory and every native harness registry.
6. Commit (closes the same-repo issue per repo baseline).

### Example: a streamable-HTTP OAuth server

`honeycomb` is the Honeycomb Intelligence MCP server at `https://mcp.honeycomb.io/mcp` (OAuth, streamable HTTP). Available on Free / Pro / Enterprise via Honeycomb Intelligence. The Enterprise gate is on the REST `/1/query_results` endpoint, not on MCP. First-time auth: `mcporter auth honeycomb`.
