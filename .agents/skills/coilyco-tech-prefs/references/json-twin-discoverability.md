# JSON-twin discoverability for dashboards

Whenever a dashboard you build has a JSON variant (whether via `Accept: application/json` content negotiation, a `?format=json` param, or a separate route), surface three discovery mechanisms so a cold-start LLM agent can find it without probing:

1. `<link rel="alternate" type="application/json" href="..." title="...">` in the HTML head.
2. `Vary: Accept` and a `Link: ...; rel="alternate"; type="application/json", ...; rel="service-desc"; type="application/json"` response header on every route.
3. A `GET /openapi.json` returning OpenAPI 3.1.

**Why:** agents land on `/` and have no way to infer JSON exists; guessing `Accept: application/json` works but is a probe, not an inference.

**How to apply:** every new internal dashboard with a machine-readable surface gets all three. Skip only when there is no JSON twin.
