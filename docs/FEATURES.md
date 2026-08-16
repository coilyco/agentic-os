# Features

Major shipped capabilities, not files.

## Inventory

- [Shell and secrets](features-shell-secrets.md) - shared shells, Warp, SSM, and GPG.
- [Branded agent terminal](alacritty-directors.md) - `aosterm` wraps `aoscompose` in native Sombra Alacritty.
- [Speech helper](aos-roles-and-voice.md) - `aos-say` client plus relay for status speech.
- **Karabiner key bindings** - external keyboard and Remote Desktop mappings.
- [Agents and sessions](features-agents.md) - self-name, composition
  status, harness policy, and
  [settings guardrails](claude-settings-guardrails.md).
- [Native agent workspaces](native-agent-workspaces.md) - fleet worktrees,
  leases, cleanup, the `aos` temp namespace, and standalone launches.
- [Agent-compose provider](agent-sdk-patterns.md) - scoped skills,
  personality, and eight deployed roles including AI Engineer.
- [Agent tool evaluation](../.agents/skills/tooling-agent-tool-evaluation/SKILL.md) - cross-harness tool evals.
- [Role-composed skills](role-composed-skills.md) - v2 Core Roster method slices.
- [AOS launcher](aos-cli.md) - role context with
  [convergence](aos-convergence.md), [connectivity](aos-standalone-connectivity.md),
  [kubeconfig](aos-kubeconfig.md), [issue pins](aos-issue-flow.md), and
  [check-ins](aos-issue-flow.md).
- [aosguard](aosguard.md) - guarded CLI with fixed
  [SigNoz MCP reads](signoz.md), issue pins, PR merge, and sealed
  [Forgejo storage measurement](forgejo-ops.md).
- [Code review skill](../.agents/composed/tooling-code-review/COMPOSED.md) - QA-only in-container review stance.
- [Code review contract](../CODE-REVIEW.md) - review invariants.
- [Test harnesses](test-harness.md) - agent/model smokes and composed-role probes.
- [Issue-corpus discovery index](issue-corpus.md) - offline corpus and live Forgejo lookup.
- [Forgejo Actions logs](forgejo-actions-logs.md) - job logs and run ZIPs.
- [Forgejo Actions list bridge](forgejo-actions-runs.md) - safe run/task listing.
- [Forgejo runner tokens](forgejo-ops.md) - guarded registration-token minting.
- [Forgejo Actions reruns](forgejo-actions-runs.md) - guarded reruns and dispatch fallback.
- [Ward integration boundary](ward-specs.md) - one generic runner for every
  [composed role](aos-roles-and-voice.md), and no role-derived authority.
- [Cross-repo tooling and release](features-release-tooling.md) - aos-precommit and release operations.
- [Telegram CI failure alerts](telegram-ci-alerts.md) - one sealed verb, no alert program in any repo.
- [dev-base image](dev-base-image.md) - parallel cached language payloads feeding one automatically released full development surface.
- [CI parity in dev-base](ci-in-dev-base.md) - CI runs inside the moving :release dev-base image.
- [Pull-request CI gate](pr-dev-base-build-validation.md) - fast tests and Docker-only image validation.
- [AGENTS pointer](features-agents.md) - generated sibling-repo workspace pointer.
- [Encoded leak guard](leak-guard.md) - hex-encoded leak-term detector.
- [Context measurement](context-budget.md) - reusable harness-neutral role
  capture, multi-provider attribution, and deterministic component diffs.
- [AGENTS inventory](agents-context-inventory.md) - fleet corpus and clipping candidates.
- [Repository residency](repo-layout.md) - Agent Compose native-workspace adapter and status tracking.
- [Catalog caps reference](catalog-caps-reference.md) - generated numeric caps for validators.
- [Canonical agent-id generator](dictatable-id-alphabet.md) - short lowercase agent IDs.
- [Agent SDK patterns](agent-sdk-patterns.md) - coordination, context, operations, tools, and managed agents.

## See also

- [README.md](../README.md) - human-facing intro.
- [AGENTS.md](../AGENTS.md) - public-safe agent operating rules.
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [features-release-tooling.md](features-release-tooling.md).
