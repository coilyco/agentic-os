# Knowledge-base freshness program

Code drift is caught loud by CI. Knowledge rot is silent: a hand-written fact (a
verb name, a model id, a price) goes stale with no commit, and a cold agent
reads it confidently wrong. Stale-and-confident is strictly worse than absent.
This program is the positive discipline for that, productized from the recovered
design thread in the recovered inbox corpus and the freshness program.

## Two axes

Grade every fact on two orthogonal axes:

- **decay-class** - how the fact is stored, and so how it rots. `asserted`
  (hand-written, highest decay, no test) -> `pointer` (states where to fetch it
  fresh, decay near zero) -> `derived` (rendered from a ground-truth source like
  `describe`/schema/`--help`, cannot drift past its source, diffable).
- **half-life** - how fast the world rewrites the fact, and so how much machinery
  it is worth. `fast` (verbs, SDK APIs, model ids, pricing, ToS) earns the
  expensive solve. `slow` (voice, doctrine, taste) stays hand-asserted, because
  maintenance cadence beats decay there and heavy machinery is waste.

## Provenance convention

A graded fact carries a machine-readable marker next to it, so an agent can
self-discount a stale claim and the probe can grade age:

```
<!-- freshness: as-of=2026-06-24 decay-class=derived half-life=fast source="..." -->
```

`as-of` (ISO date the fact was last verified) and `half-life` are required;
`decay-class` and `source` are optional. `half-life=none` keeps a fact classified
but opts it out of staleness. Markers are opt-in: ungraded files are simply not
graded, so the program rolls out incrementally.

## The probe (detection layer)

`ward freshness` ([agentic_os/freshness.py](../agentic_os/freshness.py)) parses
every marker across tracked docs and grades each by age against its half-life
horizon (`fast_days`/`slow_days` in `[tool.agentic-os.freshness]`, default 30/365).

- `ward freshness` - report every marker with its age and status.
- `ward freshness -- --check` - exit non-zero when any fact is stale or malformed.
  The loud trigger.
- `ward freshness -- --lint` - assert markers are well-formed without grading age.

[.forgejo/workflows/freshness.yml](../.forgejo/workflows/freshness.yml) runs
`--check` daily. A failed run means re-verify the flagged fact and bump its
`as-of`, or regenerate it for a derived render. The probe is hermetic (stdlib
only, no network, no binary), so it runs anywhere. It is scheduled, not a
commit-path hook, because rot is time-based, not change-based.

## Eager-context classification

The eager layer (AGENTS.md, global CLAUDE.md, skill descriptions) gets the most
discipline: every agent reads it every session, so one stale fact poisons every
run. Grading the eager facts gives the bake-unsafe / fetch-instead list:

- **fetch-instead (fast-decay, never bake)** - operator-verb names, model ids and
  pricing, public-library APIs, SSM ids. Serve from `aguard --help`, a pointer,
  or runtime lookup. The retired operator-command spellings that spawned this
  thread are exactly this cell.
- **bake-safe (slow-decay, hand-assert)** - she/her, voice rules, doctrine,
  taste. Hand-assertion in AGENTS.md is correct here.

The three storage tiers keyed to these grades (parametric / eager / runtime) and
the full ecosystem survey are in the linked design thread.

## Follow-ups

- Cold-agent assert-then-verify probe (`claude -p`, diff vs ground truth) - the
  non-hermetic half of action item 1, deferred so v1 stays deterministic.
- Auto-file an issue on a failed scheduled run, not just failing the run.
- qwen fine-tune on the slow-decay subset (action item 4), gated on this grading.

## See also

- the recovered design/research thread.
- [context-budget.md](context-budget.md) - the metered budget that makes derive-vs-point an optimization, not a blanket rule.
