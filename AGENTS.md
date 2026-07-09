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

**Operator verbs** (forgejo, aws/ssm, tailscale, kubectl, ...) live in **ward-kdl**, surfaced as `ward ops <area> ...`. Enumerate them with `ward ops <area> describe` or `--help`, or read the committed render at [`docs/ward-ops-forgejo-reference.md`](docs/ward-ops-forgejo-reference.md) - never guess an operator-verb name from prior. The old `coily ops` spelling is retired (agentic-os#261); `coily` is gone.

## Validation

This repo ships and dogfoods the catalog pre-commit suite (catalog-trifecta, documentation-layout, code-comments, catalog-block, check-skills, dead-cross-links, repo-pointer-skills, trufflehog). Run `pre-commit run --all-files` before committing. Per-repo opt-outs (excludes, cap overrides) live under `[tool.agentic-os.*]` in `pyproject.toml`.

## Safety

Keep every artifact public-safe: messages, chat, code, commits, PRs, and public text. No private identity labels in public-facing content (bios, profiles, READMEs, social, public PR text). No opaque ids, tokens, or host/network identifiers in tracked files. trufflehog runs at commit time as the secret-scan backstop, but the discipline is upstream of the hook.

## Cross-repo contracts

### Authoring vs rollout

Anything that fits as a pre-commit validation is **authored** here in agentic-os (the `agentic_os/check_*.py` validator plus its `.pre-commit-hooks.yaml` entry). Its **fleet rollout** - the thing that fans it across every checkout - lives in infrastructure/ansible, never here. The same split applies to any fleet-wide mutation: the tool or logic is authored in its home repo, and an ansible role is the rollout. Install-time mass mutation never belongs in `ward setup` or a brew post-install. Homebrew installs the binary and stops. Ansible converges the fleet.

The **trigger** for a rollout is a push, not a hand-run publish, keeping it in agent scope. When work needs the `dev-base :latest` image rebuilt (a Dockerfile, entrypoint, or pinned-`ARG` edit), the agent authors it and pushes to main - the `publish-image` job in [`release.yml`](.forgejo/workflows/release.yml) rebuilds and pushes `:latest` under the release tag. It holds no registry creds, runs no `buildx --push`: the publish is a CI consequence of the landed commit, like the tag. So "needs the image republished" is **in** scope (author, push, let CI publish), not a NO-GO wall - reserve it for a deliverable that cannot reduce to a push.

### Config placement

**Config lives at the lowest layer that fully determines it**, is consumed only by that layer or higher, and is **never fetched downward**. A shipped product never reaches up into a reference/docs repo for its own runtime config. This is the config sibling of the authoring-vs-rollout law above: logic is authored in its home layer and flows down, and so is the config that logic reads.

**Corollary** - a reference-implementation repo authors zero config that a shipped tool consumes at runtime. Fleet config that every user of the tool melds to their own values belongs down in the tool's build-time authoring layer (authored, compiled, embedded), not up in the reference repo. The reference repo may hold a clearly-marked reference copy of a config file as documentation, never a thing the tool fetches.

**Carved exception (aos#332).** ward's coilyco [`.ward/`](.ward/) spec bundle inverts this: **authored here**, overlaid into ward's release. It is Kai's **single** deployment, not the per-user meld config the corollary fences off (that stays in ward-kdl). Full reasoning: [docs/ward-specs.md](docs/ward-specs.md).

The layer gradient this keys off (churn and host-awareness rising together, a clone/use breakpoint at each): cli-guard (engine, external contributors, no upstream knowledge), then ward-kdl (the meld layer - every ward user rewrites this to their own config), then ward (the product, shipped coherent to external users), then aos (reference impl + public docs - only Kai clones it, others copy-paste from it), then infra (nobody clones it but Kai).

Config splits on three axes, each a distinct owner: **permission/surface** (ward-kdl guardfiles, dialect 1), **fleet tuning** (identity, model, endpoint, attribution, roster defaults - ward-kdl dialect 2, embedded), and **operator-local preference** (per-host, hand-edited, not embedded, parsed from a local source). One parser may serve two sources. The axes stay distinct owners.

### Skills

`.agents/skills/` ships the generalizable, public-safe skills - tooling docs for the configs that live here, plus cross-repo skills that help any agentic-os user, not just Kai. These directories are the canonical sources; harness-specific setup owns installation and discovery. Edit the SKILL.md here, not an installed copy.

## Release

Conventional-commits 1.0.0 and Forgejo issue references are encouraged house style but unenforced - the `conventional-commit` and `closes-issue` commit-msg hooks have been retired from the suite, so hand-written commits flow freely. Releases bump the minor version automatically on every push to main; the major version is hand-driven only (`scripts/release.py --bump major`), never inferred from commit messages. Canonical history lives on Forgejo; the GitHub mirror stays PR-gated. Land work on the merged branch, never `--no-verify`. Landing authority depends on the workflow mode. `direct-main` lands by merging or pushing to canonical Forgejo `main`, and a read-only container or surface session cannot do that itself. `pr` and `pull-requests` push a branch and open a human-gated Forgejo PR. `pull-requests-and-merge` opens a PR marked for the director merge lane, and the director may merge it only after the issue thread says `workflow: pull-requests-and-merge`, `WARD-OUTCOME: done`, and a passed review summary. `patch-only` has no landing authority. `ward ops forgejo pr list` is denied by policy (PRs are read through the web UI), and the GitHub mirror is the only PR-gated surface.

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

### Finish the whole task

Unless told otherwise, "done" includes the obvious follow-through, not the first reportable milestone. Finishing a task means committing, pushing to canonical main, and filing a follow-up issue for anything deferred - all of it, without returning between steps to ask. A task ends at a verifiable done-condition (tests green, the change landed, the exemption committed), not at the point where there is something to report. When the user hands off the **what**, the **what-comes-after** is part of the same job. Do not split it into separate turns that each wait on a human.

### Run until a wall worth a human

Proceed autonomously on anything reversible. Stop only for a destructive, irreversible, or externally-visible action (force-push, data loss, a post or email on the user's behalf, a public surface), or a genuine multi-path fork where the wrong choice is costly to undo. Everything short of that wall: pick the sensible default, name it inline in one line ("picking X because Y"), and keep going. A 5-second "no, do X" after the fact is cheaper than a run parked for an hour waiting on a question the user could have answered either way. Batch any genuine questions and surface them at the end with the work already done, not mid-run.

### Front-load the context you know you need

Naming a gap is not closing it. When you can already see what you do not know - a convention, a schema, how a subsystem is wired - and it sits in the repo, a skill, or a doc you can reach, read it before you plan, not lazily mid-task. "Discoverable in the clone" is a trap: it reads as resolved when the thing is only **locatable**, and an agent under pressure to start will defer the read and never come back. Before the first edit, list the conventions and subsystems the work touches and confirm you have actually read each one. The honest "the unknown is X" line in your own pre-flight is a blocking checklist item, not a footnote you walk past.

A narrowed scope does not narrow the context budget. Walking a task back from a big surface to a small one makes it look like less work and so like less to know, which is backwards. The **first** instance of a pattern needs the most grounding, not the least, because that first entry sets the schema everything after it copies. Small scope, large blast radius. When the task shrinks, reach for the existing examples harder, not less.

### Command delivery

Where a command lands depends on the **execution model**: the `/tmp` launcher convention only works when the agent's `/tmp` is the operator's.

* **Container / surface session** - a `warded` container or read-only director surface (no writable host mount). Nothing the agent writes reaches the host, so a `/tmp/<name>.sh` launcher dies with the container and never reaches the operator, reading as delivered when nothing crossed the boundary. Hand short single-line commands back **inline**. For anything multi-line, reusable, or worth tracking, commit to a **pushable** repo and push, then hand back the committed path - the only file handback that crosses the boundary from here.
* **Host harness** - native on Kai's Mac under Warp, so `/tmp` is the Mac's. The launcher guidance holds: a genuinely one-off command (pasted once, discarded) goes to `/tmp/<name>.sh` with a `bash /tmp/<name>.sh` launcher whenever it is multi-line or over 25 characters, because Warp mangles pasted multi-line and long commands. Trivial one-liners under the limit stay inline.

In either model a **reusable script** - anything Kai might run more than once, or worth tracking - is committed to a repo, never `/tmp`, and handed back as a path. This covers **any** command offered for the human to run, optional and alternative ones included, not just the primary next step. The trigger is the recipient, not the framing: commands the agent runs itself through its shell execution tool never touch a human paste path and stay out of scope.

That covers a **human** recipient. There is **no autonomous agent-to-agent command channel**. The o2r channel (the `otel-a2a-relay` relay plus its `o2r` CLI) was **archived in the June 2026 surface reduction** (`agentic-os-kai#677`), kept active but never used autonomously. Delivery is now **human-mediated**: route the request through Kai, who relays it upstream, no command crossing an agent boundary on its own. Revival and absorption are tracked at `ward#104`. The `kai-command-handover` skill holds the current procedure.

### Keep FEATURES.md current

When a change adds, removes, or materially reshapes a feature, update that repo's `docs/FEATURES.md` in the same commit. It is the living inventory completing the README / AGENTS / docs/FEATURES trifecta, not a one-time doc. A feature is significant if a user or agent would look for it there: a new subsystem, verb surface, deploy target, or capability. Mechanical refactors, bugfixes, and dependency bumps do not qualify. Pair a substantial feature with its own `docs/<feature>.md` walkthrough and link it from the FEATURES entry.

### No auto-memory

Do not write auto-memory files in any harness that offers them. Skip the save step entirely - no new files, no `MEMORY.md` updates, no edits to existing entries, even when the harness base prompt's own memory rule tells you to save. Point-in-time memory drifts: a fact true when written goes stale and then silently anchors a wrong picture in a later session, which is worse than no note at all. Keep within-session state in plans, tasks, and the conversation. Promote anything durable into an `AGENTS.md` edit (proposed for review), where the rule lives next to what it amends instead of in an unversioned side store. Reading existing memory is fine when a store is non-empty, but the default expectation is empty.

## See also

- [README.md](README.md) - human-facing intro, per-OS install steps.
- [docs/FEATURES.md](docs/FEATURES.md) - inventory of what ships today.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands. Agents route through ward, not bare `make` / `uv` / `python` / `npm` / `cargo` / `dotnet`.

Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md).
