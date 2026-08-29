# Features

Major shipped capabilities, not files.

## Inventory

- [Shell and secrets](install.md) - shared shells, Warp, SSM, and GPG.
- [Branded agent terminal](aterm.md) - `aterm` opens one composed agent session in its own Sombra kitty
  window, completing and refusing roles against the live roster, and writes a macOS `.app` launcher per
  role, each carrying its own embedded icon. The window opens on an identity card drawn from the
  overlay's own `geometry` and sounded from its `sound_mark`. Mac and Linux only, since kitty has no
  Windows build.
- [Speech helper](aos-roles-and-voice.md) - `aos-say` client plus relay for status speech.
- **Karabiner key bindings** - external keyboard and Remote Desktop mappings.
- [Agents and sessions](features-agents.md) - self-name, composition
  status, harness policy, and
  [settings guardrails](native-claude-credentials.md).
- [Native agent workspaces](native-agent-workspaces.md) - role-scoped worktrees,
  leases, cleanup, the `aos` temp namespace, and standalone launches.
  [Shadow lifecycle](native-shadow.md) verbs list, release, and reap them.
- [Fail-closed launch provenance](native-session-start.md) - native startup
  fetches each policy source, verifies the digest Agent Compose sealed into the
  repository plan, regenerates once on a mismatch, and stops before any worktree
  exists when that cannot converge.
- [Agent-compose provider](context-budget.md) - scoped skills,
  personality, and seven deployed roles across the Agent Compose v3 roster.
- [Agent tool evaluation](../.agents/skills/tooling-agent-tool-evaluation/SKILL.md) - cross-harness tool evals.
- [Role-composed skills](role-composed-skills.md) - v2 Core Roster method slices.
- [AOS launcher](aos-cli.md) - role context with
  [convergence](aos-convergence.md), [connectivity](aos-context-bundle.md),
  [kubeconfig](aos-cluster-access.md), [issue pins](aos-issue-flow.md), and
  [check-ins](aos-issue-flow.md).
- [aosguard](aosguard.md) - guarded CLI with fixed
  [SigNoz MCP reads](signoz.md), issue pins, PR merge, and sealed
  [Forgejo storage measurement](forgejo-ops.md).
- [Code review skill](../.agents/composed/tooling-code-review/COMPOSED.md) - Portfolio Director gate-decision review stance.
- [Code review contract](../CODE-REVIEW.md) - review invariants.
- [Forgejo Actions logs](forgejo-actions-runs.md) - job logs and run ZIPs.
- [Forgejo Actions list bridge](forgejo-actions-runs.md) - safe run/task listing.
- [Forgejo runner tokens](forgejo-ops.md) - guarded registration-token minting.
- [Forgejo Actions reruns](forgejo-actions-runs.md) - guarded reruns and dispatch fallback.
- [Ward integration boundary](ward-specs.md) - one generic runner for every
  [composed role](aos-roles-and-voice.md), and no role-derived authority.
- [Cross-repo tooling and release](release.md) - aos-precommit and release operations.
- [Telegram CI failure alerts](signoz.md) - one sealed verb, no alert program in any repo.
- [dev-base image](dev-base-image.md) - parallel cached language payloads feeding one automatically released full development surface.
- [CI parity in dev-base](ci-in-dev-base.md) - CI runs inside the moving :release dev-base image.
- [Pull-request CI gate](ci-in-dev-base.md) - fast tests and Docker-only image validation.
- [AGENTS pointer](features-agents.md) - generated sibling-repo workspace pointer.
- [AGENTS git-workflow block](features-agents.md) - generated per-lane standing authorization to commit, branch, push, and open a PR.
- [Encoded leak guard](pre-commit-hygiene.md) - hex-encoded leak-term detector.
- [Outbound link hygiene](pre-commit-hygiene.md) - offline validator for links
  leaving the repo, driven by a retired-name and retired-path table, plus a
  report-only liveness CLI for a scheduled job.
- [Managed line endings](pre-commit-hygiene.md) - generated `.gitattributes`
  block pinning the working tree to LF, with vendored trees and vendor orgs
  left alone.
- [Context measurement](context-budget.md) - reusable harness-neutral role
  capture, multi-provider attribution, and deterministic component diffs.
- [AGENTS inventory](agents-context-inventory.md) - fleet corpus and clipping candidates.
- [Repository residency](repo-layout.md) - Agent Compose native-workspace adapter and status tracking.
- [Catalog caps reference](catalog-caps-reference.md) - generated numeric caps for validators.
- [Canonical agent-id generator](build-output-is-not-content.md) - short lowercase agent IDs.
- [Agent-compose provider](context-budget.md) - the AOS capability provider contract.

## See also

- [README.md](../README.md) - human-facing intro.
- [AGENTS.md](../AGENTS.md) - public-safe agent operating rules.
- [justfile](../justfile) - dev verbs.
- [.ward/ward.yaml](../.ward/ward.yaml) - catalog metadata only.

Cross-reference convention from [release.md](release.md).
