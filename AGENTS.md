# Agent instructions

This file carries the public-safe, universal conventions (pronouns, voice, name-the-actor, command delivery) composed into each supported agent harness's global context on public and work hosts. It also completes the symmetric trifecta (README / AGENTS / docs/FEATURES) and stays grep-discoverable.

## Scope

This is the public-safe operating base, read on every session on public and work hosts. It holds the universal conventions that apply to any agentic-os user, not just Kai. Harness-specific and private sources may add context after this base. Keep this file public-safe: no private identity labels, no opaque ids, no host-specific secrets. Push host-specific or private detail into an appropriate scoped source, not here.

## Project shape

Kai calls this repo **aos** for short (chat and issue refs). `aos` and `agentic-os` refer to the same thing - the GitHub slug stays `agentic-os`. The repo ships the cross-repo pre-commit hooks (the catalog suite), the public-safe skills under `.agents/skills/`, and supporting subsystems (`warp/` Go module).

## Repo boundaries

Public hosts and work laptops import this base only. Personal machines may compose additional scoped sources after it. Edit each canonical source, not generated output or an installed copy. This repo is the source of the catalog hooks; consumer repos reference it by upstream ref, never fork the validators.

## Commands

Route every dev command through ward, which reads [`.ward/ward.yaml`](.ward/ward.yaml). Agents invoke `ward <verb>`, not bare `make` / `uv` / `python` / `npm` / `cargo` / `dotnet`. Add new verbs to that file before invoking them.

**Operator verbs** (forgejo, aws/ssm, tailscale, kubectl, ...) live in **aosguard**, surfaced as `aosguard ops <area> ...` from the full dev-base image. Enumerate them with `aosguard ops <area> describe` or `--help` - never guess an operator-verb name from prior. Ward retains fixed workflow policy and repository development commands. The old `coily ops` and human-facing `ward ops` spellings are retired.

## Validation

This repo ships and dogfoods the catalog pre-commit suite (catalog-trifecta, documentation-layout, code-comments, catalog-block, check-skills, check-composed-skills, dead-cross-links, repo-pointer-skills, trufflehog). Run `pre-commit run --all-files` before committing. Per-repo opt-outs (excludes, cap overrides) live under `[tool.agentic-os.*]` in `pyproject.toml`.

**Tests never encode or reinterpret configuration.** A tunable lives in one
owning source. Do not add consumer-, service-, or repo-specific test programs
whose assertions restate a guardfile, KDL file, manifest, workflow, or other
configuration. The owning loader tests its behavior with fixtures and validates
real configuration through its schema, lint, render, or doctor surface (`ward
doctor` gates `.ward` in ci and promote). Consumer repos invoke that surface
instead of building a second parser or contract test. CI derives inventories
from the owning loader or a wildcard instead of maintaining a duplicate list.

## Safety

Keep every artifact public-safe: messages, chat, code, commits, PRs, and public text. No private identity labels in public-facing content (bios, profiles, READMEs, social, public PR text). No opaque ids, tokens, or host/network identifiers in tracked files. trufflehog runs at commit time as the secret-scan backstop, but the discipline is upstream of the hook.

## Cross-repo contracts

### Authoring vs rollout

Anything that fits as a pre-commit validation is **authored** here in agentic-os (the `agentic_os/check_*.py` validator plus its `.pre-commit-hooks.yaml` entry). Its **fleet rollout** - the thing that fans it across every checkout - lives in infrastructure/ansible, never here. The same split applies to any fleet-wide mutation: the tool or logic is authored in its home repo, and an ansible role is the rollout. Install-time mass mutation never belongs in `ward setup` or a brew post-install. Homebrew installs the binary and stops. Ansible converges the fleet.

The **trigger** for a rollout is a push, not a hand-run publish, keeping it in agent scope. When work needs the dev-base image family rebuilt (a Dockerfile, entrypoint, or pinned-`ARG` edit), the agent authors it and pushes to main - the `dev-base-publish` workflow builds the parallel Ubuntu language images and their full fan-in image, and the release workflow promotes their tags plus the moving full-image `:release` alias. It holds no registry creds, runs no `buildx --push`: the publish is a CI consequence of the landed commit, like the tag. So "needs the image republished" is **in** scope (author, push, let CI publish), not a NO-GO wall - reserve it for a deliverable that cannot reduce to a push.

### Config placement

**Config lives at the lowest layer that fully determines it**, is consumed only by that layer or higher, and is **never fetched downward**. A shipped product never reaches up into a reference/docs repo for its own runtime config. This is the config sibling of the authoring-vs-rollout law above: logic is authored in its home layer and flows down, and so is the config that logic reads.

**Corollary** - a reference-implementation repo authors zero config that a shipped tool consumes at runtime. Fleet config that every user of the tool melds to their own values belongs down in the tool's build-time authoring layer (authored, compiled, embedded), not up in the reference repo. The reference repo may hold a clearly-marked reference copy of a config file as documentation, never a thing the tool fetches.

**Deployment boundary (aos#778).** AOS owns agent-compose inputs, harness
selection, deployment identity, and standalone AOSguard policy. Ward owns fixed
workflows and its broker. AOS does not ship a Ward role-policy or KDL bundle.
Only the supported YAML in [`.ward/ward.yaml`](.ward/ward.yaml) remains for
repository command and fixture declaration. Full reasoning:
[docs/ward-specs.md](docs/ward-specs.md).

The layer gradient this keys off (churn and host-awareness rising together, a clone/use breakpoint at each): cli-guard and specgen (generic engines, external contributors, no upstream knowledge), then Ward (fixed workflows and broker), then aos (AOSguard policy, composition inputs, and public docs), then infra (nobody clones it but Kai).

Config splits on three axes, each a distinct owner: **permission/surface**
(AOSguard specs and Ward's fixed broker), **deployment tuning** (identity,
model, endpoint, attribution, roster defaults - AOS and agent-compose launch
inputs), and **operator-local preference** (per-host, hand-edited, not
embedded, parsed from a local source). One parser may serve two sources. The
axes stay distinct owners.

### Skills

`.agents/skills/` ships generalizable, public-safe ordinary skills.
`.agents/composed/` ships public-safe role-scoped sources that agent-compose
promotes only after role selection. These directories are canonical.
Harness-specific setup owns installation and discovery. Edit `SKILL.md` or
`COMPOSED.md` here, not an installed copy.

## Release

Conventional-commits 1.0.0 and Forgejo issue references are encouraged house style but unenforced - the `conventional-commit` and `closes-issue` commit-msg hooks have been retired from the suite, so hand-written commits flow freely. Releases bump the minor version automatically on every push to main; the major version is hand-driven only (`scripts/release.py --bump major`), never inferred from commit messages. Canonical history lives on Forgejo; the GitHub mirror stays PR-gated. Never `--no-verify`. `ward agent` headless dispatch follows the resolved workflow:

* `direct-to-main` - merge or push to `main`, then close the issue.
* `pull-request` - push a branch and open a human-gated Forgejo PR.
* `pull-request-and-merge` - open a PR for the director lane. Merge only after the issue thread shows `workflow: pull-request-and-merge`, `WARD-OUTCOME: done`, and a passed review summary.
* `remote-branch-only` - push a branch and stop. No PR or merge authority.

A read-only clone cannot push itself, so push or merge workflows need a writable surface. Track landed work by issue state and commits on `main`. `aosguard ops forgejo pr list` and `pr view` are allowed. Merge stays gated.

## Agent rules

### Pronouns

**She/her always.** Never he/him or they/them in any artifact for Kai - messages, chat, code, commits, PRs, public text. Default she/her when ambiguous; fix legacy they/them on contact, except in marked historical records.

### Voice rules

* No em-dashes - use periods, commas, parens, or ` - `.
* No italics - bold only, for structural anchors.
* No semicolons in prose.
* No prose tables - flat bullets `* <anchor> - <cats> - <details>`.

### Name the actor

In every action sentence, name who performs it: "Kai commits them" or "the agent commits them", never "I'll commit them" (ambiguous). This matters most in user-input option labels - every choice presented to Kai says whose hands are on it.

### Action-first communication

Shape every response so the reader can act without retaining hidden state. This
is baseline communication, not an opt-in mode.

* Lead with the outcome or next action. Skip filler preambles.
* Number human-executed multi-step work. Keep the immediate list to five bounded actions, then split later work.
* Keep state visible across turns. Name what finished, the current step, and one next action without repeating a plan already visible in a task tool.
* Finish the main thread before introducing tangents. State errors matter-of-factly, make completed work visible, and give concrete time ranges only when they help and the uncertainty is named.
* End with one concrete next action when work remains. Otherwise end when the answer is complete, without a boilerplate closer.

The task and safety rules outrank the output shape. Explain fully when asked,
confirm before destructive action, ask one focused question when ambiguity is
material, and stop a three-turn debug spiral to name the suspect assumption.
Required harness announcements still happen.

Adapted from [`i-have-adhd`](https://github.com/ayghri/i-have-adhd) (MIT), which
frames the conventions as broadly useful without requiring a diagnosis.

### Finish the whole task

Unless told otherwise, "done" includes the obvious follow-through, not the first reportable milestone. Finishing a task means committing, pushing to canonical main, and filing a follow-up issue for anything deferred - all of it, without returning between steps to ask. A task ends at a verifiable done-condition (tests green, the change landed, the exemption committed), not at the point where there is something to report. When the user hands off the **what**, the **what-comes-after** is part of the same job. Do not split it into separate turns that each wait on a human.

### Native checkpoints must be remote

A native host session writes into a long-lived checkout. Whenever an agent doing native work outside `warded` reaches a checkpoint with local repository changes, the agent **must commit the in-scope changes and push the commit to the canonical remote before pausing, reporting the checkpoint, switching tasks, or ending the turn**. A checkpoint includes a human decision wall, a blocked dependency, a handoff, a context boundary, and any point where the agent may not continue immediately.

If the resolved workflow allows the work to land, the agent pushes it to `main` as usual. If the work should not yet land on `main`, the agent creates or reuses a task-specific branch and pushes the checkpoint there. The remote branch is the recovery artifact. Uncommitted changes, local-only commits, stashes, reflogs, and a clean local worktree without a remote ref **do not count**. Test failures or incomplete follow-up may keep a checkpoint off `main`, but they never justify leaving the only copy local. Never force-push to satisfy this rule. If an ordinary push cannot succeed, the agent preserves the local state and reports the exact blocker as the current wall.

### Foreign work requires a worktree

Before the first mutation in any native checkout, the agent inspects the worktree, current branch, and local divergence. If the checkout contains work the agent did not create for the current task, the agent **must not edit, format, stage, stash, reset, switch branches, commit, or otherwise mutate that checkout**. The agent creates a separate task-specific branch and linked worktree from the appropriate clean canonical base, then performs all current-task work inside that worktree. The agent places the linked worktree inside the local operating system's resolved temporary directory, with a task-specific basename, rather than beside the canonical checkout or elsewhere in the project workspace. Pre-existing staged, unstaged, or untracked files, local commits, an in-progress Git operation, and a branch owned by another task all count as foreign work. Ambiguous ownership counts as foreign.

The agent leaves the original checkout exactly as found. The agent never absorbs foreign changes into its commit or uses stash as a substitute for isolation. If the agent cannot create a separate worktree safely, the agent stops before any mutation and reports the exact blocker. The remote-checkpoint rule applies to the new worktree and its task branch.

### Unlisted repository clones stay temporary

Before cloning a repository, the agent checks the host's expected-repositories
list when that surface is configured. If the repository is not explicitly
listed as one that belongs on disk, the agent clones it into the local
operating system's resolved temporary directory with a task-specific basename,
never under the persistent projects or workspace tree. The agent treats that
clone as task-scoped and removes it when the work is complete and the
remote-checkpoint requirements are satisfied. An absent or unreadable list
does not authorize a persistent checkout.

### Human-only workdirs

A checkout whose directory basename ends in `-workdir` is reserved for Kai's manual work. Agents treat it as outside the workspace: agents do not inspect, enter, edit, validate, format, stage, stash, or include it in fleet or recursive tooling. If an agent launches inside one, the agent stops before inspecting repository contents and moves to the canonical checkout or an agent-owned linked worktree.

### Run until a wall worth a human

Proceed autonomously on anything reversible. Stop only for a destructive, irreversible, or externally-visible action (force-push, data loss, a post or email on the user's behalf, a public surface), or a genuine multi-path fork where the wrong choice is costly to undo. Everything short of that wall: pick the sensible default, name it inline in one line ("picking X because Y"), and keep going. A 5-second "no, do X" after the fact is cheaper than a run parked for an hour waiting on a question the user could have answered either way. Batch any genuine questions and surface them at the end with the work already done, not mid-run.

### Engineers and QA: never debug or iterate against live operations

This rule binds the **sealed roles - engineer and QA**. Their ephemeral clones are sealed against live mutation, not approved read-only observation. Director and ops retain their wider operational surfaces and remediation authority.

An engineer or QA may inspect approved read-only observability surfaces, including logs, traces, metrics, health, events, resource state, and rollout status. The engineer may use directly observed evidence for diagnosis, and QA may use it in a verdict. Neither role may execute commands inside workloads, inspect secrets or raw customer payloads, mutate live systems, deploy, or iterate against production. When the next diagnostic or verification step needs a live action beyond observation, the role names the exact operator action and expected evidence, then stops at that boundary.

CI/CD is live operations for this rule. The engineer or QA may read workflow logs, summarize evidence, and make one locally grounded push for a change whose behavior the repo proves. Repeated pushes to probe Forgejo Actions, release promotion, package registries, runner configuration, Actions secrets, rollout jobs, or deployment pipelines are ops debugging. If the failure only appears in live CI/CD or registry state, the engineer or QA stops after gathering evidence, files an `interactive`-labeled issue with the exact failing run and needed live verification, and hands it to a director or ops run.

Deploys already have established precedent (exposure patterns, exemplar services, shared charts). For deploy work: **match the precedent and copy the exemplar, do not invent or iterate.** The engineer or QA may report health, log, trace, metric, and rollout evidence visible through an approved read-only surface. Neither role initiates a deployment or live verification action. When verification needs such an action, the engineer or QA files an `interactive`-labeled issue describing exactly what the operator must do and what evidence must return, then hands it to the operator, director, or ops. Do not push a speculative fix and hope CI confirms it.

Cross-reference the deploy precedent doc (`coilyco-bridge/deploy/docs/deploy-patterns.md`, forthcoming) and the burndown repo-exclusion filter (`coilyco-flight-deck/ward#1105`).

### Front-load the context you know you need

Naming a gap is not closing it. When a convention, schema, or subsystem wiring is discoverable in the repo, a skill, or a doc, read it before planning. Before the first edit, list the conventions and subsystems the work touches and confirm you have read each one.

A narrowed scope does not narrow the context budget. The **first** instance of a pattern needs the most grounding, because that first entry sets the schema everything after it copies.

### Command delivery

Commands for a human operator must cross the current execution boundary truthfully.

* **Container / surface session** - a `warded` container or read-only director surface has no writable host mount. Hand one-off commands back inline. For anything reusable or worth tracking, commit to a **pushable** repo and push, then hand back the committed path. A local container file does not cross this boundary.
* **Host harness** - hand one-off commands back inline. A temporary file is optional when it materially improves review or safety, not a terminal-specific paste requirement.

In either model a **reusable script** - anything Kai might run more than once, or worth tracking - is committed to a repo and handed back as a path. This covers **any** command offered for the human to run, optional and alternative ones included, not just the primary next step. The trigger is the recipient, not the framing: commands the agent runs itself through its shell execution tool never touch a human paste path and stay out of scope.

That covers a **human** recipient. There is **no autonomous agent-to-agent command channel**. The o2r channel (the `otel-a2a-relay` relay plus its `o2r` CLI) was **archived in the June 2026 surface reduction** (`agentic-os-kai#677`), kept active but never used autonomously. Delivery is now **human-mediated**: route the request through Kai, who relays it upstream, no command crossing an agent boundary on its own. Revival and absorption are tracked at `ward#104`. The `kai-command-handover` skill holds the current procedure.

### Keep FEATURES.md current

When a change adds, removes, or materially reshapes a feature, update that repo's `docs/FEATURES.md` in the same commit. It is the coarse inventory of major shipped capabilities, not a changelog or bugfix ledger. Add an entry only for a new or removed significant capability: a subsystem, command family, deploy target, major integration, or broad user-facing behavior. For an existing capability, update the entry only when the public boundary materially changes. Do not update FEATURES for bugfixes, diagnostics or error-message fixes, validation hardening, CI or build fixes, dependency bumps, refactors, docs-only changes, internal plumbing, or small behavior changes. Put those details in the specific docs page, PR or issue, release notes, or code comments. Pair a substantial feature with its own `docs/<feature>.md` walkthrough and link it from the FEATURES entry.

### No auto-memory

Do not write auto-memory files in any harness that offers them. Skip the save step entirely - no new files, no `MEMORY.md` updates, no edits to existing entries, even when the harness base prompt's own memory rule tells you to save. Point-in-time memory drifts: a fact true when written goes stale and then silently anchors a wrong picture in a later session, which is worse than no note at all. Keep within-session state in plans, tasks, and the conversation. Promote anything durable into an `AGENTS.md` edit (proposed for review), where the rule lives next to what it amends instead of in an unversioned side store. Reading existing memory is fine when a store is non-empty, but the default expectation is empty.

## See also

- [README.md](README.md) - human-facing intro, per-OS install steps.
- [docs/FEATURES.md](docs/FEATURES.md) - inventory of what ships today.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands. Agents route through ward, not bare `make` / `uv` / `python` / `npm` / `cargo` / `dotnet`.

Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md).
