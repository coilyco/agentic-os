# Composed role question probes

`aos-role-question` proves that a projected role can answer through a real model,
not only that its files exist. Each invocation launches one disposable AOS
container, asks one role-specific question, and requires the answer to identify
the role loaded from harness instructions.

## Mechanisms

* **Cloud** - Codex receives the host auth file read-only, copies it into the
  ephemeral HOME, and runs read-only with no persisted session.
* **Local** - Goose receives its host connection config read-only and runs the
  selected Ollama model without a session.
* **Storage** - the composed agent HOME and `/tmp` are bounded tmpfs mounts. The
  no-substrate provider clone lives under `/tmp`, so initialization consumes no
  shared Docker writable-layer budget and hides no image-owned `/home` tools.
* **Validation** - each run has a ten-minute ceiling and fails unless its output
  contains `ROLE-CONFIRMED: <role>`. Matching ignores case and treats spaces and
  hyphens in role names as equivalent.

The optional third local argument overrides the model for a single probe. The
default is the local design-production model.

Build the launcher and local image before the first run:

```bash
ward exec aos-build
ward exec aos-image-build
```

## Cloud designer

```bash
ward exec aos-role-question -- cloud designer
```

## Local role matrix

Run these sequentially:

```bash
ward exec aos-role-question -- local engineer
ward exec aos-role-question -- local director
ward exec aos-role-question -- local qa
ward exec aos-role-question -- local advisor
ward exec aos-role-question -- local ops
ward exec aos-role-question -- local pm
ward exec aos-role-question -- local designer
ward exec aos-role-question -- local social
ward exec aos-role-question -- local sales
ward exec aos-role-question -- local customer-success
```

Each question targets the role's actual work, such as adversarial verification
for QA, remediation triage for ops, or interaction shaping for designer. The
probe never supplies the expected role name in the prompt.

## First complete run

On 2026-07-23, authenticated designer Codex and all ten Goose role combinations
returned the expected role marker and a substantive answer. An exploratory
OpenCode path reached the selected model but produced no completion within ten
minutes, while direct inference stayed healthy. Goose remains the operational
local one-shot harness for this matrix.

## Related

* [aos-cli.md](aos-cli.md) - composed-container launch contract.
* [test-harness-goose.md](test-harness-goose.md) - general Goose probe mechanics.
* [role-composed-skills.md](role-composed-skills.md) - role-gated skill delivery.
