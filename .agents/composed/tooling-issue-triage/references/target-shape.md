# Target shape

**`priority/P0` has no quota - it is content-based, in two steps: net then confirm.**

1. **Net (recall, deterministic):** a script scans each issue's title+body for P0 signals - secret/token leak, arbitrary code execution or auth bypass, data loss, active outage/crashloop, broken deploy pipeline, "blocks committed work". The exact patterns live in [references/p0-content-rules.yaml](p0-content-rules.yaml). This casts a wide net.
2. **Confirm (precision, judgment):** keyword rules over-match badly (~40% of hits are *about* a topic, not incidents *of* it). So confirm each candidate with a one-line judgment call: **"active incident / live exposure, or just discussing it?"** Keep only the active ones - a bounded per-candidate decision a small local model can own.

You never force a `priority/P0` percentage - urgent is whatever genuinely is (a re-triage of ~750 issues confirmed ~19).

The **non-`priority/P0` remainder** splits to a global distribution across the resolved
priority pool, as equal-width ranges so the cut lands on a natural break, not a
forced percentage: **`priority/P1` 0-20%, `priority/P2` 10-30%, `priority/P3` 20-40%, `priority/P4` 30-50%** - same
20-point width, centers at **10 / 20 / 30 / 40**, summing to 100. Treat the
band, not a single number, as the target. `priority/P1` floors at zero on purpose: a
backlog with nothing important-and-near-term has an empty `priority/P1`, and that is
correct. Small or urgent repositories may deviate past a band edge. The shape
holds on the resolved pool.
