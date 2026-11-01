# The dispatch gate

The enforcing half is a ceiling gate at the shared dispatch chokepoint: a surface runs an issue only when `surface <= ceiling` on the order `headless > live-collab > async-consult`. It lives in `umbra`'s `resolveDispatchIssue`, the chokepoint every surface flows through, reading the issue's autonomy label off `Issue.Labels` (populated by ward's GitHub fetcher, and by its Forgejo fetcher until that retired with Forgejo on 2026-11-01, COI-2931). The autonomous surfaces gate at the headless ceiling. Unlabeled fails closed.

**The gate matches label names, so renaming them is a breaking change** - see [label-taxonomy](label-taxonomy.md). Note also that the **surface** names and the **label** names are no longer one vocabulary. They were aligned before the 2026-08-15 rename and are not now, so a matcher cannot treat them as interchangeable.
