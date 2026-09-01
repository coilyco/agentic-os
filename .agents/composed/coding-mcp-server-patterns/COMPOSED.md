---
name: coding-mcp-server-patterns
description: Build or review Node and TypeScript MCP servers with tools, resources, prompts, schemas, and stdio or Streamable HTTP transports. Triggers - MCP server, MCP tool, MCP resource, MCP prompt, MCP transport, Model Context Protocol.
metadata:
  origin: ECC
  source: https://github.com/affaan-m/ECC/tree/591ab5cbd3f2f65860ea91c226e410b1502c8e2e/skills/mcp-server-patterns
  revision: 591ab5cbd3f2f65860ea91c226e410b1502c8e2e
  license: MIT
---

# MCP Server Patterns

Use this skill when an agent builds, reviews, upgrades, or diagnoses an MCP
server. MCP SDK APIs evolve. Verify method names and signatures against the
current official MCP specification and SDK documentation before authoring code.

## Select the capability surface

Confirm that MCP is the right wall before implementing it:

* **Tool** - an action the model may invoke.
* **Resource** - read-only context the client may fetch.
* **Prompt** - a reusable parameterized interaction the client may surface.
* **Plain CLI or API** - a deterministic workflow that does not benefit from
  model-facing discovery.

Keep executable authority outside the skill. Apply the target repo's
permission, command-routing, validation, and secret-handling rules.

## Form the server

1. Separate domain logic from MCP registration and transport code.
2. Define an input schema and documented output form for every tool.
3. Prefer resources for read-only data and tools for actions.
4. Return concise structured errors that help the model recover. Do not expose
   stack traces, secrets, or raw private payloads.
5. Make mutating tools idempotent where practical. Document retry, rate, cost,
   and confirmation behavior.
6. Keep tool names stable, descriptions task-oriented, and result payloads
   bounded.
7. Pin the SDK version and check its release notes before upgrading.

Node and TypeScript servers commonly use `@modelcontextprotocol/sdk` and Zod,
but the exact registration API has changed across versions. Some releases use
`registerTool()` and `registerResource()`, while others expose shorter methods.
Use the current SDK documentation as the source of truth.

## Choose a transport

* **stdio** - use for a local process launched and supervised by the client.
  Write protocol messages only to stdout and send diagnostics to stderr.
* **Streamable HTTP** - use for remote or shared servers. Apply authentication,
  origin validation, session handling, timeouts, request limits, and safe
  shutdown at the HTTP wall.
* **Legacy HTTP and SSE** - retain only when an existing client requires
  backward compatibility.

Keep tool and resource handlers independent of the transport so a server can
change entrypoints without rewriting domain behavior.

## Verify the implementation

Exercise:

* registration and discovery for every tool, resource, and prompt
* valid and invalid schema inputs
* expected errors, timeouts, cancellation, and retry behavior
* authentication and authorization boundaries for remote servers
* clean startup and shutdown for the chosen transport
* bounded results for large or adversarial inputs
* compatibility with the actual target client

Use the target repo's Ward or equivalent command surface for setup,
formatting, tests, and builds. Do not copy a package-manager command into the
workflow when the repo owns a wrapper.
## Provenance

Adapted from [`affaan-m/ECC/skills/mcp-server-patterns`](https://github.com/affaan-m/ECC/tree/591ab5cbd3f2f65860ea91c226e410b1502c8e2e/skills/mcp-server-patterns)
at revision `591ab5cbd3f2f65860ea91c226e410b1502c8e2e`. MIT licensed. See [`LICENSE`](LICENSE).
