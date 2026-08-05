# Features

Major shipped capabilities, not files.

## Inventory

- [Shell and secrets](features-shell-secrets.md) - shared shells, Warp, SSM, and GPG.
- [Branded director terminal](alacritty-directors.md) - native Sombra Alacritty launcher.
- [Speech helper](aos-say.md) - `aos-say` client plus relay for status speech.
- **Karabiner key bindings** - external keyboard and Remote Desktop mappings.
- [Agents and sessions](features-agents-sessions.md) - self-name, composition status, and harness policy.
- [Native agent workspaces](native-agent-workspaces.md) - fleet worktrees,
  leases, cleanup, the `aos` temp namespace, and native agent launches.
- [Agent-compose provider](personality-provider.md) - scoped skills and
  personality under the `aos` source namespace.
- [Agent tool evaluation](../.agents/skills/tooling-agent-tool-evaluation/SKILL.md) - cross-harness tool evals.
- [Role-composed skills](role-composed-skills.md) - v2 Core Roster method slices.
- [AOS launcher](aos-cli.md) - role and context with
  [convergence](aos-convergence.md),
  [MCP and tailnet](aos-standalone-connectivity.md),
  [role-gated kubeconfig](aos-kubeconfig.md), and
  [check-ins](aos-acompose-checkin.md).
- [aosguard](aosguard.md) - guarded CLI with fixed one-tool
  [SigNoz MCP reads](signoz.md) and fixed-target issue pins.
- [Code review skill](../.agents/composed/tooling-code-review/COMPOSED.md) - QA-only in-container review stance.
- [Code review contract](../CODE-REVIEW.md) - review invariants.
- [Test harnesses](test-harness.md) - agent/model smokes and composed-role probes.
- [Issue-corpus discovery index](issue-corpus.md) - offline corpus and live Forgejo lookup.
- [Forgejo Actions logs](forgejo-actions-logs.md) - job logs and run ZIPs.
- [Forgejo Actions list bridge](forgejo-actions-listing.md) - safe run/task listing.
- [Forgejo runner tokens](forgejo-runner-token.md) - guarded registration-token minting.
- [Forgejo Actions reruns](forgejo-actions-rerun.md) - guarded reruns and dispatch fallback.
- [Ward integration boundary](ward-specs.md) - workflows, arbitrary
  [composed roles](aos-generic-warded-roles.md), and no role-derived authority.
- [QA verification fixture](qa-verification-fixture.md) - bounded live role proof.
- [Cross-repo tooling and release](features-release-tooling.md) - aos-precommit and release operations.
- [Telegram CI failure alerts](telegram-ci-alerts.md) - reusable red-channel alerting for failing jobs.
- [dev-base image](dev-base-image.md) - parallel cached language payloads feeding one released full development surface.
- [CI parity in dev-base](ci-in-dev-base.md) - CI runs inside the moving :release dev-base image.
- [Pull-request CI gate](pr-dev-base-build-validation.md) - fast tests and Docker-only image validation.
- [AGENTS pointer](features-agents-pointer.md) - generated sibling-repo workspace pointer.
- [Encoded leak guard](leak-guard.md) - hex-encoded leak-term detector.
- [Context budget](context-budget.md) - role-seat budgets, multi-provider attribution, and generated class diffs.
- [AGENTS inventory](agents-context-inventory.md) - fleet corpus and clipping candidates.
- [Mount eligibility](mount-eligibility-manifest.md) - per-harness repository allowlist.
- [Catalog caps reference](catalog-caps-reference.md) - generated numeric caps for validators.
- [Canonical agent-id generator](dictatable-id-alphabet.md) - short lowercase agent IDs.
- [Knowledge-base freshness](knowledge-base-freshness.md) - age-based fact markers.
- [Agent SDK patterns](agent-sdk-patterns.md) - coordination, context, operations, tools, and managed agents.

## See also

- [README.md](../README.md) - human-facing intro.
- [AGENTS.md](../AGENTS.md) - public-safe agent operating rules.
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [features-release-tooling.md](features-release-tooling.md).
