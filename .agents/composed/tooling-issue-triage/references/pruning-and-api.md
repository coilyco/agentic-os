# Pruning and running it over an API

## Pruning - demote, merge, or close

Default to **demoting to `priority/P4` (icebox)** rather than closing: speculative-but-kept work stays open and tracked at the lowest tier, and the async triage loop can only move issues between open tiers anyway. Reserve closing for two cases:

- **Merge** near-duplicates into the lowest-numbered canonical (comment "merged into #N", then close the losers).
- **Hard close** only the genuinely dead - superseded, abandoned, or one-line stubs with no value. For a bulk burn-down, an `icebox` label on the closed issue keeps it reversible (`state:closed label:icebox`).
- **Keep** anything concrete, a bug, infra/security/ops, committed-direction, OR anything uncertain - at its earned tier. Keep is the safe default; demotion to `priority/P4` is the soft prune, closing is the hard one.

## Running it over an API - lessons

- **Route writes through ward - it resolves the canonical repo path for you.** The hazard: after a repo transfer or rename the old path 301-redirects, and most HTTP clients follow the redirect on GET (reads succeed) but convert POST/PATCH/DELETE to GET and drop the body - the write silently no-ops and returns 200, looking like success. ward now canonicalizes the repo path before every write, so this is handled when you go through it. Only when you bypass ward and hit the API raw do you still need to fetch the repo first, read its post-redirect canonical name, and issue every write against that.
- **Give fan-out triage agents a hard coverage mandate.** Per-repo agents reliably under-paginate and stop at roughly half a repo's issues. Hand each one its exact open count (the total-count response header) and require it to retrieve all N or fail.
- **Count from the per-repo issues endpoint, not a cross-repo search.** Cross-repo issue-search totals can over-count (e.g. counting moved/duplicate rows); the per-repo issues endpoint's total-count header is the trustworthy number for ratio math.
- **If your issue CLI lacks label add/remove verbs** (only label-definition CRUD), set per-issue labels via the API until those verbs exist.
- **An exclusive label group swaps in one call.** Adding `autonomy/headless` to an issue carrying `autonomy/async-consult` removes the old value by itself, so no paired remove is needed. A non-exclusive group like `role/*` does need the pair.
- **Removing a label takes its numeric id, not its name.** `aosguard ops forgejo issue-label remove <owner> <repo> <index> <identifier>` reads `identifier` as the label id. Passing the name returns success and removes nothing, which is the silent-failure shape to watch for: verify with `issue-label list` rather than trusting the exit code.
