# Phase 3 - Categorize and rank

Categorize entries semantically (eg. game-server-ops, gmail, calendar,
gaming, observability, dev-tools, social, finance, ai-infrastructure,
speculative-asks, etc.). Then rank globally - not within categories -
at 3:2:1 ratio: 🥉 50%, 🥈 33%, 🥇 17%. A sparse category may end up
entirely 🥉. That's expected and intentional.

Ranking criteria, in priority order:

1. Direct fit to current repo work or open issues across the human's repo set.
2. Reduces a manual workflow the human is currently doing by hand.
3. Reusable across multiple projects.
4. Speculative entries the human has personal leverage on (someone they know)
   rank higher than ones they don't.

**Do not rank skills by skillsmp `stars`.** The star count reflects the
**host repo**, not the individual skill. Aggregator collections (eg.
antigravity-awesome-skills at ~40k stars) inflate every contained skill's
star count equally, so a skill bundled into a popular collection looks far
more endorsed than it is. Judge skills by **author reputation + description
fit** instead, and treat stars as host-repo signal only, never per-skill
endorsement.

Prepend the medal emoji to each entry. Output:
`YYYY-MM-DD-capability-scout-3-ranked.yaml`. Group by category, but
keep the global rank labels intact.
