# Handbook: Status lines and required sections

## 4. The status line (under H1, where enforced)

Status enforcement is per-category. See
[`categories.yaml`](../../../skills/categories.yaml) for which categories
enforce it. The format when enforced:

```markdown
# <Title>

Status: <emoji> <Kind> | Last <updated|tested>: YYYY-MM-DD
```

The emoji is part of the canonical format and pairs one-to-one with the kind. Validator rejects any other pairing.

Currently enforced:

* `ops-investigation-*` - kinds: `🟢 Active`, `⚪ Stub`, `🛠 Runbook`, `📋 CaseStudy`. Freshness: `Last updated`.
* `ops-investigation` (router) - kind: `🗺 Router`. Freshness: `Last updated`.

Status-kind sub-shapes for `ops-investigation-*`:

* **Active.** The default. Real, live investigation guide. Required H2 sections enforced.
* **Stub.** Placeholder, will be expanded. Only `Overview` required. That section explains why it's a stub and where the work will land.
* **Runbook.** Operational rollout/runbook. Free-form body beyond `Overview`.
* **CaseStudy.** Single-incident worked example. Free-form beyond `Overview`. Cross-link the underlying pattern.

Categories without status enforcement: free-form. Add a status line voluntarily if it pulls weight (e.g., a daily-* skill noting when its data sources last changed shape), but the validator does not require it.

## 5. Required H2 sections per category

Validator-enforced where listed. Names are exact (case-insensitive, leading/trailing whitespace ignored). Order is recommended but not enforced. Categories not listed below are free-form.

### `ops-investigation-*` (Status: Active)

H1 must match `^# .+ Investigation Guide$` for `Active` and `Stub`. `Runbook` and `CaseStudy` H1s are free-form.

```markdown
## Overview
## Data sources
## Investigation procedure
## Common patterns
## Common dead ends
```

Notes:

* `Overview` answers "what is this thing?" in 1-3 paragraphs.
* `Data sources` lists the systems / collections / APIs / log streams you'll be reading.
* `Investigation procedure` is the step-by-step.
* `Common patterns` covers recurring failure modes / known categories.
* `Common dead ends` is what NOT to do, with reasons.

Optional sections (allowed, not required): `Architecture`, `Rollout phases`, `Monitoring runbook`, etc.

### Routers (`ops-investigation`)

```markdown
## Routing table
```

Plus any cross-cutting rules that apply to every routed-to skill. The router is NOT the place for procedure that's specific to one routed-to skill.

### All other categories

Free-form. Frontmatter still enforced.

### Repo AGENTS.md heading set

The catalog-trifecta validator enforces a standard H2 set for every repo-local `AGENTS.md`: `## Scope`, `## Project shape`, `## Repo boundaries`, `## Commands`, `## Validation`, `## Safety`, `## Cross-repo contracts`, `## Release`, `## Agent rules`, `## See also`.

These headings are intentionally broad. If a repo has nothing special for a section, keep the heading and write one line saying the repo inherits the workspace default or has no repo-local rule. The point is fast agent scanning, not forcing every repo to invent policy.
