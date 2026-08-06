# Role-skill coverage audit

The 2026-08 second pass reviewed every AOS ordinary and composed source against
all nine deployed Agent Compose roles.

## Placement result

The 12 ordinary sources remain ordinary. Each is a cross-role tool adapter whose
trigger and operating boundary are useful regardless of the selected role.

Every composed source is selected by at least one role except the two sales
methods. AOS has no deployed sales role, and those B2B commercial methods do not match
the career or portfolio-investment boundary closely enough to burden Strats.

## Corrections

* **Director** - narrows coding guidance to the decision-relevant core and
  architecture shapes, while adding the PM family used for product decisions.
* **QA** - retains `coding-core-supply-chain-audit` because dependency trust is
  an independent-verification concern.
* **Design** - gains the PM signal-triangulation and program-decomposition methods
  alongside product brainstorming.
* **Community** - gains the social editorial loop alongside customer-signal,
  trust-repair, cultural-reading, trust-boundary, and Discord methods.
* **Content** - drops issue decomposition because converting plans into tracker
  work belongs with decision and strategy roles rather than content production.
* **AI Engineer** - receives the coding family used to build benchmark runners,
  probes, aggregation tools, and other AI-measurement artifacts.

`.agents/roles.kdl` remains the authoritative selection configuration. This page
records rationale only, so it does not become a second role matrix.
