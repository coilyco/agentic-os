# Assignment method (the part that actually works)

Distributed per-repo judgment cannot hit a global ratio on its own - it overshoots. Here, global means the resolved [priority pool](priority-pool.md): one repository by default, or an explicitly declared organization portfolio. So **carve `priority/P0` by rule, then let judgment provide the ordering and percentile enforce the shape** on the rest:

1. **Carve `priority/P0` first**: run the content rules (net) over title+body, then judgment-confirm each candidate (see [target-shape](target-shape.md)). The confirmed set is `priority/P0` and leaves the pool.
2. Score the remaining pool by urgency. Robust signal: run two independent judgment passes (a triage cascade twice, or two rubrics), map `priority/P1` to 3, `priority/P2` to 2, `priority/P3` to 1, then sum. Unsure -> `priority/P3` (the default).
3. Rank the remainder globally by score (tiebreak on the later/corrected pass, then issue number) and cut by percentile into the target bands: top **0-20%** -> `priority/P1`, next **20-40%** -> `priority/P2`, and the remainder -> `priority/P3`, which is uncapped because it is the default. Within each band, put the cut at the nearest natural score break rather than a forced exact percentile - that is the whole point of a range. `priority/P1` floors at zero: if nothing clears the important-and-near-term bar, leave `priority/P1` empty rather than promoting a `priority/P2` to fill a quota.

Sanity-check that the `priority/P0` content rules actually catch the dangerous ones (credential leaks, arbitrary-code-execution, crashloops, broken deploys) - those are the failures you cannot afford to miss-tier.

The agents' relative urgency calls survive, and the distribution lands within the target bands. Sanity-check that obviously-urgent issues (credential leaks, arbitrary-code-execution, crashloops, broken deploys) land in `priority/P0`.

## Autonomy comes from the thread, not the title

The tier can be cut by percentile. The autonomy value cannot - it is a per-issue judgment, and it has to **read the comment thread**. A pass over titles and bodies alone over-promotes badly: a measured re-verification of 32 issues labelled headless found 16 wrong, and twelve of those had been set back to consult by the very seat that did the work, in a comment the pass never opened. The thread is where a design fork gets named, where a seat releases a claim, and where "this needs you" is written down. A promotion made without reading it is a guess.
