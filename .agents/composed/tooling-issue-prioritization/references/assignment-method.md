# Assignment method (the part that actually works)

Distributed per-repo judgment cannot hit a global ratio on its own - it overshoots. Here, global means the resolved priority pool from the skill entrypoint: one repository by default, or an explicitly declared organization portfolio. So **carve P0 by rule, then let judgment provide the ordering and percentile enforce the shape** on the rest:

1. **Carve P0 first**: run the content rules (net) over title+body, then judgment-confirm each candidate (see Target shape in the skill entrypoint). The confirmed set is P0 and leaves the pool.
2. Score the remaining pool by urgency. Robust signal: run two independent judgment passes (a triage cascade twice, or two rubrics), map `P1=3 P2=2 P3=1 P4=0`, sum. Unsure -> P3 (the default).
3. Rank the remainder globally by score (tiebreak on the later/corrected pass, then issue number) and cut by percentile into the target bands (equal width 20, centers evenly spaced on 10/20/30/40): top **0-20%** -> P1, next **10-30%** -> P2, next **20-40%** -> P3, bottom **30-50%** -> P4. Within each band, put the cut at the nearest natural score break rather than a forced exact percentile - that is the whole point of a range. P1 floors at zero: if nothing clears the important-and-near-term bar, leave P1 empty rather than promoting a P2 to fill a quota.

Sanity-check that the P0 content rules actually catch the dangerous ones (credential leaks, arbitrary-code-execution, crashloops, broken deploys) - those are the failures you cannot afford to miss-tier.

The agents' relative urgency calls survive, and the distribution lands within the target bands. Sanity-check that obviously-urgent issues (credential leaks, arbitrary-code-execution, crashloops, broken deploys) land in P0.
