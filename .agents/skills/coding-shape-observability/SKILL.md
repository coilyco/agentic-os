---
name: coding-shape-observability
description: Category umbrella for observability work across vendors - metrics, traces, logs, dashboards, alerting, SLOs. Current edge is OpenTelemetry plus LLM consumers of observability data.
---

# coding-shape-observability

Umbrella for any observability work. Spans metrics, traces, logs, dashboards, alerting, SLOs. Kai's professional spine across multiple employers: Datadog, Prometheus, Grafana, Sentry, CloudWatch, New Relic. Current edge is OTel plus LLM consumers of observability data.

## Triggers

observability, o11y, monitoring, metrics, traces, logs, datadog, prometheus, grafana, sentry, cloudwatch, new relic, opentelemetry, otel, fluent-bit, fluentd, vector, loki, tempo, jaeger, victoriametrics.

## Vendor familiarity (from resume)

- **Datadog** - Textio (custom Datadog + Fluent-bit setup), Bluelink-adjacent.
- **New Relic** - prior platform roles.
- **Prometheus + Grafana** - homelab default, also Textio-adjacent.
- **Sentry** - personal stack, every coilysiren/* service ships a DSN.
- **CloudWatch** - whenever AWS-native is the right reach.
- **OpenTelemetry** - current edge of work.

## Defaults

- **Metrics**: Prometheus exposition format. RED method for services (Rate, Errors, Duration), USE for resources (Utilization, Saturation, Errors).
- **Traces**: OpenTelemetry. Span-per-tool-call when instrumenting agent code. LLM-consumer substrates read those spans via VictoriaMetrics + per-session buffers.
- **Logs**: structured JSON. One event per line. Correlation IDs in every line.
- **Dashboards**: Grafana for personal, vendor-native (Datadog/New Relic) when an employer pays for it.
- **Alerting**: SLO-driven. Burn-rate alerts on multi-window, multi-burn-rate (Google SRE workbook).
- **Errors**: Sentry. DSN per service in SSM (`/sentry-dsn/<project>` convention). Static exception messages, dynamic context on attributes - never interpolate unbounded values into the message string.

## LLM consumers are first-class

Kai's current edge: design observability for LLM consumers, not human dashboards. Raw event streams accessible via MCP. MCP tools consume the substrate directly, not just humans visiting Grafana.

When designing new observability surfaces, ask: how does an LLM agent consume this? If the answer is "open Grafana and look", that's incomplete.

## Anti-patterns

- Logging at every function boundary (logspam, useless).
- Metrics without dashboards (write-only telemetry).
- Dashboards without alerts (read-only telemetry).
- Alerts without runbooks (paging without action).
- Per-employer vendor lock that doesn't transfer (favor OTel and standard exposition formats).
- High-cardinality values (file paths, IDs, URLs, user input) in exception messages. Sentry groups and titles on the message, so a per-file message shatters one logical error into thousands of issues and breaks "group similar". Keep the message a closed-set string, push the dynamic data into structured exception attributes or Sentry `extra` / `fingerprint`. Low-cardinality enums (codec, status) are fine to inline.

## When this skill is active

Designing instrumentation, building dashboards, configuring alerts, debugging via observability surfaces, or wiring a new service into the o11y stack.

## See also

- `agentic-os-kai/SSM.md` - `/sentry-dsn/*` parameter inventory.
