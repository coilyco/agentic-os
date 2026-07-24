---
name: coding-shape-web-server
description: Category umbrella for building HTTP / REST / GraphQL servers. Python via FastAPI (Flask on legacy), Go via stdlib net/http or chi, Node via Fastify. Async I/O, OpenAPI, Prometheus from day one.
---

# coding-shape-web-server

Umbrella for any work that ships an HTTP server as the primary interface.

## Triggers

web server, http server, rest api, graphql, fastapi, flask, django, sanic, starlette, gin, chi, echo, fiber, express, fastify, hapi, koa, openapi, swagger.

## Framework defaults by language

- **Python** → `FastAPI`. Async-first, type-driven, OpenAPI built in. Reach for Flask only on legacy codebases.
- **Go** → stdlib `net/http` for simple, `chi` for routing-heavy. Skip gin/echo/fiber unless project commits to them.
- **TypeScript** → Fastify > Express. Node-side servers should default to async/await throughout.

## Design principles

- **OpenAPI from day one.** `/openapi.json` exists, accurate, kept in sync with handlers. Cross-link to the JSON-twin discoverability rule in `kai-tech-prefs` (in agentic-os-kai).
- **Async I/O when there's I/O.** Sync handlers only when there's nothing to overlap.
- **Structured errors.** Return shaped JSON for 4xx/5xx, not bare strings.
- **Prometheus metrics from day one.** `/metrics` endpoint, request duration histograms, error counters. See [`coding-shape-observability`](../../skills/coding-shape-observability/SKILL.md).
- **Healthcheck split.** `/healthz` for liveness, `/readyz` for readiness. Don't conflate.
- **CORS configured explicitly.** Wide-open `*` is usually wrong.
- **Auth at the edge.** Middleware/gateway, not per-handler.

## Anti-patterns

- Sync handlers for I/O-bound work.
- Custom routing instead of using the framework's.
- Hand-rolled OpenAPI written separately from the handlers.
- Stateful servers when statelessness was achievable.

## When this skill is active

Building a new HTTP server, refactoring an existing one, or designing the API surface. Cross-link to the language skill.

## See also

- [`coding-shape-observability`](../../skills/coding-shape-observability/SKILL.md) - metrics/tracing wiring.
- [`coding-kubernetes`](../../skills/coding-kubernetes/SKILL.md) - deploy target for most servers Kai writes.
- `kai-tech-prefs` (in agentic-os-kai) - JSON-twin discoverability rule.
