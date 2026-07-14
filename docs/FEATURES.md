# Features

What `agentic-os` does. Cross-platform shell, terminal, and secret-handling.

This doc lists major shipped capabilities, not files. If you want a file inventory, run `ls`.

## Inventory

- [Shell and secret handling](features-shell-secrets.md) - shared shell setup, Warp config, SSM secrets, GPG bootstrap, repo gate.
- [Speech helper](aos-say.md) - `aos-say` client plus relay for status speech.
- **Karabiner key bindings** - complex modifications for the external keyboard and Remote Desktop.
- [Agents and sessions](features-agents-sessions.md) - agent self-name, session pulse, and status-line repo tracking.
- [Code review skill](../.agents/skills/tooling-code-review/SKILL.md) - the in-container review stance for ward workers.
- [Code review contract](../CODE-REVIEW.md) - the root review doctrine for repo-local invariants and refresh triggers.
- [Harness selection](harness-selection.md) - choose Claude, Codex, OpenCode, Aider, or Goose.
- [Test harnesses](test-harness.md) - smoke tests for agent harness and model pairings.
- [Issue-corpus discovery index](issue-corpus.md) - offline corpus render plus live Forgejo lookup.
- [Forgejo Actions log bridge](forgejo-actions-logs.md) - packaged plaintext helper for live workflow logs while ward#950 moves the surface to ward-kdl.
- [Forgejo Actions list bridge](forgejo-actions-listing.md) - safe first-page helper for live Actions run/task inspection while raw `limit` examples stay pinned to `page=1`.
- [Forgejo Actions rerun bridge](forgejo-actions-rerun.md) - guarded run-id rerun helper for failed Forgejo Actions jobs on the coilyco deployment.
- [ward deployment spec bundle](ward-specs.md) - the shipped `WARD_CONFIG_REF` bundle for the coilyco fleet.
- [Ward profile assets home](ward-profile-assets.md) - AOS-owned profile/config source layout for Ward's typed `ProfileProvider` seam.
- [Role surface tiers](role-surface-tiers.md) - the intended per-role container capability tiers.
- [Cross-repo tooling and release](features-release-tooling.md) - hooks, diagnostics, and Forgejo-canonical release actions, with the release pipeline gated before tagging.
- [Telegram CI failure alerts](telegram-ci-alerts.md) - reusable red-channel alerting for failing jobs.
- [dev-base container image](dev-base-image.md) - the inner-loop toolchain image family, including the self-name banner and the coilyco-ops git committer stamp.
- [CI parity in dev-base](ci-in-dev-base.md) - CI runs inside the pinned dev-base image.
- [Pull-request CI gate](ci-in-dev-base.md) - the live `ci` workflow that runs `pytest` on pull requests.
- [Managed AGENTS.md pointer block](features-agents-pointer.md) - generated workspace pointer block for sibling repos.
- [Encoded leak guard](leak-guard.md) - hex-encoded leak-term detector.
- [Context-budget report](context-budget.md) - eager startup budget measurement per harness.
- [Mount-eligibility manifest](mount-eligibility-manifest.md) - per-harness repo mount list for this host.
- [Committed ward-ops reference](ward-ops-forgejo-reference.md) - checked-in `ward ops forgejo` command render.
- [Catalog caps reference](catalog-caps-reference.md) - generated numeric caps for validators.
- [Tool-failure GlitchTip shipper](tool-failures-shipper.md) - batch shipper for ward-owned tool failure records, with schema-v1 docs still owned here.
- [Canonical agent-id generator](dictatable-id-alphabet.md) - short lowercase agent IDs.
- [Knowledge-base freshness program](knowledge-base-freshness.md) - age-based doc fact freshness markers.

## See also

- [README.md](../README.md) - human-facing intro.
- [AGENTS.md](../AGENTS.md) - public-safe agent operating rules.
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [features-release-tooling.md](features-release-tooling.md).
