# Features

Major capabilities shipped by `agentic-os`.

This lists shipped capabilities, not files.

## Inventory

- [Shell and secrets](features-shell-secrets.md) - shared shell setup, Warp, SSM, GPG, and repo gate.
- [Branded director terminal](alacritty-directors.md) - Sombra Alacritty director, projects-root default, and cwd override.
- [Alacritty workspace](zellij-workspace.md) - persistent Zellij, vertical tabs, panes, and direct shortcuts.
- [Speech helper](aos-say.md) - `aos-say` client plus relay for status speech.
- **Karabiner key bindings** - complex modifications for the external keyboard and Remote Desktop.
- [Agents and sessions](features-agents-sessions.md) - self-name, session pulse, status line, and harness policy.
- [Agent-compose provider](personality-provider.md) - scoped skills, community hosting, harness registry, and personality alignment.
- [Role-composed skills](role-composed-skills.md) - role-gated specialist and coding knowledge.
- [AOS launcher](aos-cli.md) - Go containers, cloud and local projections, and [composed-role check-ins](aos-acompose-checkin.md).
- [aguard](aguard.md) - canonical guarded operator CLI in the full image and paired native AOS releases.
- [Code review skill](../.agents/composed/tooling-code-review/COMPOSED.md) - QA-only in-container review stance.
- [Code review contract](../CODE-REVIEW.md) - review invariants.
- [Harness selection](harness-selection.md) - 11-role execution-routing and personality-alignment boards.
- [Test harnesses](test-harness.md) - agent/model smokes and composed-role probes.
- [Issue-corpus discovery index](issue-corpus.md) - offline corpus render plus live Forgejo lookup.
- [Forgejo Actions log bridge](forgejo-actions-logs.md) - plaintext helper for live workflow logs, plus fetch mirror.
- [Forgejo Actions list bridge](forgejo-actions-listing.md) - safe first-page helper for Actions run/task inspection.
- [Forgejo runner-token fetch overlay](forgejo-runner-token.md) - guarded runner registration-token minting via fetch leaves.
- [Forgejo Actions rerun bridge](forgejo-actions-rerun.md) - guarded reruns with dispatch fallback and fetch mirror.
- [ward bundle](ward-specs.md) - guarded surfaces, AOSH seat identity and Goose routing, AOS-local OpenCode policy.
- [Ward profile assets home](ward-profile-assets.md) - AOS profile/config inputs for Ward's `ProfileProvider`.
- [Role surface tiers](role-surface-tiers.md) - guarded Engineer/QA observation and wider Director/Ops tiers.
- [Cross-repo tooling and release](features-release-tooling.md) - hooks, diagnostics, promotion, retries, reruns.
- [Telegram CI failure alerts](telegram-ci-alerts.md) - reusable red-channel alerting for failing jobs.
- [dev-base image](dev-base-image.md) - one full Ubuntu image with every language toolchain.
- [CI parity in dev-base](ci-in-dev-base.md) - CI runs inside the moving :release dev-base image.
- [Pull-request CI gate](ci-in-dev-base.md) - the live `ci` workflow that runs `pytest` on pull requests.
- [AGENTS pointer](features-agents-pointer.md) - generated sibling-repo workspace pointer.
- [Encoded leak guard](leak-guard.md) - hex-encoded leak-term detector.
- [Context budget](context-budget.md) - role-seat budgets and generated class diffs.
- [AGENTS inventory](agents-context-inventory.md) - fleet corpus and cascades.
- [Mount eligibility](mount-eligibility-manifest.md) - per-harness repository allowlist.
- [Catalog caps reference](catalog-caps-reference.md) - generated numeric caps for validators.
- [Canonical agent-id generator](dictatable-id-alphabet.md) - short lowercase agent IDs.
- [Knowledge-base freshness program](knowledge-base-freshness.md) - age-based doc fact freshness markers.

## See also

- [README.md](../README.md) - human-facing intro.
- [AGENTS.md](../AGENTS.md) - public-safe agent operating rules.
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [features-release-tooling.md](features-release-tooling.md).
