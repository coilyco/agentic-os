---
ward:
  workflow: pull-request-and-merge
---
# Agent instructions

This file carries the public-safe, universal conventions (pronouns, voice, name-the-actor, command delivery) composed into each supported agent harness's global context on public and work hosts. It also completes the symmetric trifecta (README / AGENTS / docs/FEATURES) and stays grep-discoverable.

## Scope

This is the public-safe operating base, read on every session on public and work hosts. It holds the universal conventions that apply to anyone using agentic-os, not just Kai. Harness-specific and private sources may add context after this base. Keep this file public-safe: no private identity labels, no opaque ids, no host-specific secrets. Push host-specific or private detail into an appropriate scoped source, not here.

## Project shape

Kai calls this repo **aos** for short (chat and issue refs). `aos` and `agentic-os` refer to the same thing - the GitHub slug stays `agentic-os`. The repo ships the cross-repo pre-commit hooks (the catalog suite), the public-safe skills under `.agents/skills/`, and supporting subsystems (`warp/` Go module).

## Repo boundaries

Public hosts and work laptops import this base only. Personal machines may compose additional scoped sources after it. Edit each canonical source, not generated output or an installed copy. This repo is the source of the catalog hooks; consumer repos reference it by upstream ref, never fork the validators.

## Commands

Route every dev command through the [`justfile`](justfile). Agents invoke `just <verb> <args>`, not bare `make` / `uv` / `python` / `npm` / `cargo` / `dotnet`. Bare `just` lists every verb. Add new verbs to that file before invoking them.

**Operator verbs** (forgejo, aws/ssm, tailscale, kubectl, ...) live in **aosguard**, surfaced as `aosguard ops <area> ...` from the full dev-base image. Enumerate them with `aosguard ops <area> describe` or `--help` - never guess an operator-verb name from prior. Ward retains fixed workflow policy. Repository development commands are the justfile's. The old `coily ops` and human-facing `ward ops` spellings are retired.

**Model transport** goes through Agent Proxy. Whenever a task would invoke
Ollama or LiteLLM, use Agent Proxy's OpenAI-compatible surface instead because
both are backends behind it. Direct evaluations targeting those backends send
the frozen model request through Agent Proxy without launching an agent
harness. Harness evaluations may launch the harness under test, but transport
to those backends still routes through Agent Proxy. Backend-direct calls are
limited to Agent Proxy implementation, parity testing, or incident isolation,
and the caller names the exception explicitly.

**Public cloud evaluation approval.** Repository behavior evaluations are
pre-authorized to send public-safe, tracked repository role bundles,
evaluation prompts, rubrics, and model responses to OpenAI's Codex cloud for
model inference and independent review. This authorization excludes secrets,
credentials, private overlays, customer data, unpublished personal
information, and raw operational payloads. It authorizes evaluation transport
only, not publication or any other external action.

## Validation

This repo ships and dogfoods the catalog pre-commit suite (catalog-trifecta, documentation-layout, code-comments, check-skills, check-composed-skills, dead-cross-links, repo-pointer-skills, trufflehog). Run `pre-commit run --all-files` before committing. Per-repo opt-outs (excludes, cap overrides) live under `[tool.agentic-os.*]` in `pyproject.toml`.

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

The **trigger** for a rollout is a push, not a hand-run publish, keeping it in agent scope. When work needs the dev-base graph rebuilt (a Dockerfile, entrypoint, or pinned-`ARG` edit), the agent authors it and pushes to main - the `dev-base-publish` workflow's native `docker/**` path filter selects the promoted change, then Forgejo's matrix and `needs` graph build parallel cached Ubuntu language payloads and their full fan-in image. Commit-scoped payload manifests are build transport only. The release workflow promotes the full image's version tag plus its moving `:release` alias. It holds no registry creds, runs no `buildx --push`: the publish is a CI consequence of the landed commit, like the tag. So "needs the image republished" is **in** scope (author, push, let CI publish), not a NO-GO wall - reserve it for a deliverable that cannot reduce to a push.

### Config placement

**Config lives at the lowest layer that fully determines it**, is consumed only by that layer or higher, and is **never fetched downward**. A shipped product never reaches up into a reference/docs repo for its own runtime config. This is the config sibling of the authoring-vs-rollout law above: logic is authored in its home layer and flows down, and so is the config that logic reads.

**Corollary** - a reference-implementation repo authors zero config that a shipped tool consumes at runtime. Fleet config that everyone using the tool melds to their own values belongs down in the tool's build-time authoring layer (authored, compiled, embedded), not up in the reference repo. The reference repo may hold a clearly-marked reference copy of a config file as documentation, never a thing the tool fetches.

**Deployment boundary (aos#778).** AOS owns agent-compose inputs, harness
selection, deployment identity, and standalone AOSguard policy. Ward owns fixed
workflows and its broker. AOS does not ship a Ward role-policy or KDL bundle.
Only the supported YAML in [`.ward/ward.yaml`](.ward/ward.yaml) remains, and it
carries catalog metadata since inbox#366 moved dev verbs. Full reasoning:
[docs/ward-specs.md](docs/ward-specs.md).

The layer gradient this keys off (churn and host-awareness rising together, a clone/use breakpoint at each): umbra and specgen (generic engines, external contributors, no upstream knowledge), then Ward (fixed workflows and broker), then aos (AOSguard policy, composition inputs, and public docs), then infra (nobody clones it but Kai).

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

Conventional-commits 1.0.0 and Forgejo issue references are encouraged house style but unenforced - the `conventional-commit` and `closes-issue` commit-msg hooks have been retired from the suite, so hand-written commits flow freely. The standalone AOS CLI bumps its minor version automatically only when a push to main changes a shipped binary or package input. The `aos-precommit-v*` train advances only when the promoted diff changes an installed hook input. Dev-base publication runs only when the promoted diff affects an image tier. Manual workflow dispatch remains the explicit retry or override path. Major versions are hand-driven only (`scripts/release.py --bump major` for aos-precommit, workflow dispatch for other trains) and are never inferred from commit messages. Canonical history lives on Forgejo; the GitHub mirror stays PR-gated. Never `--no-verify`. `ward agent` headless dispatch follows the resolved workflow:

* `merge-remote-main` - merge or push to `main`, then close the issue. Ward's default lane.
* `pull-request` - push a branch and open a human-gated Forgejo PR.
* `pull-request-and-merge` - open a PR for the director lane. Merge only after the issue thread shows `workflow: pull-request-and-merge`, `WARD-OUTCOME: done`, and a passed review summary.
* `remote-branch-only` - push a branch and stop. No PR or merge authority.

A read-only clone cannot push itself, so push or merge workflows need a writable surface. Track landed work by issue state and commits on `main`. `aosguard ops forgejo pr list` and `pr view` are allowed. Merge stays gated.

## Agent rules

<!-- BEGIN managed by agentic-os/scripts/apply-git-workflow.py -->
### Git workflow

**This repo runs the `pull-request-and-merge` lane**, declared as `ward.workflow` in this file's frontmatter. The agent commits to a task branch, pushes it, opens a Forgejo pull request, and **merges that pull request itself** once it is green. The author of the code is the one who merges it. Opening the pull request is a step, never the stopping point.

The fleet runs two lanes, and both authorize the same core actions:

* `merge-remote-main` - the agent commits, pushes to `main`, and closes the issue. No branch and no pull request.
* `pull-request-and-merge` - the agent commits to a task branch, pushes it, opens a pull request, and merges that pull request itself once it is green.

**Every lane slug names what the AGENT does, never what someone else does.** `pull-request-and-merge` carries the merge because the agent that authored the code merges its own pull request. `pull-request` drops `-and-merge` because the author stops at the pull request and the director merge lane takes over. Reading `pull-request-and-merge` as "someone else merges it later" inverts the two lanes and leaves finished work sitting unmerged.

**These actions are pre-authorized on every lane, and the agent MUST take them without asking first.** Committing, creating a branch, pushing a branch, pushing the lane's own destination, and opening a pull request are ordinary reversible work, not the destructive wall that earns a question. Stopping to ask is how a turn ends with the work stranded in a dirty worktree.

* **ALWAYS commit** in-scope work and **ALWAYS push** it to the canonical remote before pausing, reporting a checkpoint, handing off, or ending a turn. A local-only commit is not a checkpoint.
* **ALWAYS open the pull request** in the same turn as the branch's first push, on every lane except `remote-branch-only`. A pushed branch with no pull request is litter nobody reviews.
* **NEVER `--no-verify`** and **NEVER force-push**. Those two are the real walls, and they stay closed.
* **ALWAYS merge your own pull request on `pull-request-and-merge`**, in the same turn, as soon as it is green. Reporting it as open and awaiting someone is the failure this lane exists to prevent.
* **NEVER merge on `pull-request` or `remote-branch-only`.** Those two stop where they stop, and the director merge lane carries a `pull-request` from there.
<!-- END managed by agentic-os/scripts/apply-git-workflow.py -->

### Who you are talking to

This base is composed for whoever is in front of it, and that is not always
Kai. Two nouns cover every person a rule here can mean, and no rule invents a
third.

* **the human** - whoever the agent is talking to this session. The noun makes no authority claim, so authority stays governed by the workflow and runtime language the rules already use.
* **peer** - a counterparty that is not a human at all.

A sentence about the estate needs no noun for a person. It drops the name and
describes the thing. Sentences about Kai's portfolio, repositories,
preferences, and house style name Kai and stay true regardless of who is
driving. Every ask, accept, decide, choose, and confirm is about the human, and
is the kind of sentence that breaks when the human is a stranger.

### Pronouns

**Kai is she/her, always.** Never he/him or they/them for Kai in any artifact -
messages, chat, code, commits, PRs, public text. Fix legacy they/them on
contact, except in marked historical records. Inside a reference to Kai,
ambiguity resolves to she/her.

The rule is about Kai and reaches nobody else. **Anyone whose pronouns you have
not been told is they/them**, the human in front of you included, along with
any third party the work names. A name is not a source for pronouns. Ambiguity
about whether the subject is Kai resolves the other way: if you have not
established that the subject is Kai, use they/them.

### Voice rules

* No em-dashes and no `·` separators - use periods, commas, parens, ` - `, or ` // `. This covers rendered agent output, not only prose. Rendered rows and titles take ` // `, matching the identity cards.
* No italics - bold only, for structural anchors.
* No semicolons in prose.
* No prose tables - flat bullets `* <anchor> - <cats> - <details>`.

### Speak as yourself

In direct conversation, use first person for your own actions: "I checked the
logs" or "I'll commit the change." Use your resolved seat name only when
identity materially matters. Reserve "the agent" for generic agents or explicit
multi-agent distinctions. Name the human when the human acts, and name Kai
when the sentence is about Kai. Keep ownership equally explicit in
question option labels and handoffs: say "I'll implement it" or "the human
will choose," not an actorless imperative. A selected voice specialty may
deliberately impose a different grammatical perspective.

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

Unless told otherwise, "done" includes the obvious follow-through, not the first reportable milestone. Finishing a task means committing, pushing to canonical main, and filing a follow-up issue for anything deferred - all of it, without returning between steps to ask. A task ends at a verifiable done-condition (tests green, the change landed, the exemption committed), not at the point where there is something to report. When the human hands off the **what**, the **what-comes-after** is part of the same job. Do not split it into separate turns that each wait on a human.

### Native checkpoints must be remote

Whenever an agent doing native work outside `warded` reaches a checkpoint with local repository changes, the agent **must commit the in-scope changes and push the commit to the canonical remote before pausing, reporting the checkpoint, switching tasks, or ending the turn**. A checkpoint includes a human decision wall, a blocked dependency, a handoff, a context boundary, and any point where the agent may not continue immediately.

If the resolved workflow allows the work to land, the agent pushes it to `main` as usual. If the work should not yet land on `main`, the agent creates or reuses a task-specific branch and pushes the checkpoint there. The remote branch is the recovery artifact. Uncommitted changes, local-only commits, stashes, reflogs, and a clean local worktree without a remote ref **do not count**. Test failures or incomplete follow-up may keep a checkpoint off `main`, but they never justify leaving the only copy local. Never force-push to satisfy this rule. If an ordinary push cannot succeed, the agent preserves the local state and reports the exact blocker as the current wall.

### A pushed branch owes its pull request

The rule above ends at a remote branch, and **a branch is not a deliverable**. Whenever an agent pushes a branch on any lane that lands through review (`pull-request`, `pull-request-and-merge`), it **opens the pull request in the same turn**, before reporting the checkpoint, handing back, or ending the turn. A branch pushed on a lane that lands on `main` still owes one whenever the merge itself is blocked, because the branch is then the only thing carrying the work. Only `remote-branch-only` stops at a branch, and only when the caller resolved that lane.

**A branch with no PR is litter**: nothing points at it, no review is pending on it, and it is invisible to anyone reading issues or the merge queue, so the work is lost the moment the session ends. When the agent cannot open the PR itself, it reports that as the blocking wall and hands back the branch name with its compare URL rather than describing the work as pushed and done.

### Native session shadow

A native AOS launch runs the agent in a per-session shadow, not the canonical checkout. `AOS_NATIVE_SESSION` and `AOS_NATIVE_SESSION_PROJECTS` are set exactly when it exists, so the agent reads them rather than guessing. The shadow shares canonical Git objects, so a commit is durable at once while its working tree stays exposed to temporary-root purges, the mechanism behind the rule above. Placement and mechanics: [session shadow](docs/native-shadow.md).

### Foreign work requires a worktree

This rule governs a session with no native shadow. Before the first mutation in any native checkout, the agent inspects the worktree, current branch, and local divergence. If the checkout contains work the agent did not create for the current task, the agent **must not edit, format, stage, stash, reset, switch branches, commit, or otherwise mutate that checkout**. The agent instead takes a task-specific branch and linked worktree from a clean canonical base, leaves the original checkout exactly as found, and never absorbs foreign changes or substitutes stash for isolation. Pre-existing staged, unstaged, or untracked files, local commits, an in-progress Git operation, and a branch owned by another task all count as foreign, as does ambiguous ownership. If the agent cannot create that worktree safely, it stops before any mutation and reports the exact blocker.

### Unlisted repository clones stay temporary

Before cloning a repository, the agent checks the host's expected-repositories
list when that surface is configured. If the repository is not explicitly
listed as one that belongs on disk, the agent clones it into the resolved
temporary directory with a task-specific basename, never under the persistent
projects or workspace tree. The agent treats that clone as task-scoped and
removes it once the work is complete and remote-checkpoint requirements are
satisfied. An absent or unreadable list does not authorize a persistent
checkout.

### Serialized checkouts invert isolation

A repository in the serialized set inverts the three rules above. The agent works it in the canonical checkout under `$PROJECTS_ROOT`, never in a session shadow, a linked worktree, or a temporary clone, and treats it as belonging on disk whether or not the residency plan lists it. A serialized project holds an editor or world lock that one writer takes at a time, so a second tree resolves that same lock and merges back as corruption rather than as a conflict Git can show. Before its first mutation the agent confirms that nothing else holds the checkout, and stops and reports when something does rather than branching around it. The set and its host scope: [native agent workspaces](docs/native-agent-workspaces.md).

### Human-only workdirs

A checkout whose directory basename ends in `-workdir` is reserved for Kai's manual work. Agents treat it as outside the workspace: agents do not inspect, enter, edit, validate, format, stage, stash, or include it in fleet or recursive tooling. If an agent launches inside one, the agent stops before inspecting repository contents and moves to the canonical checkout or an agent-owned linked worktree.

### Run until a wall worth a human

Proceed autonomously on anything reversible. Stop only for a destructive, irreversible, or externally-visible action (force-push, data loss, a post or email on the human's behalf, a public surface), or a genuine multi-path fork where the wrong choice is costly to undo. Everything short of that wall: pick the sensible default, name it inline in one line ("picking X because Y"), and keep going. A 5-second "no, do X" after the fact is cheaper than a run parked for an hour waiting on a question the human could have answered either way. Batch any genuine questions and surface them at the end with the work already done, not mid-run.

### Front-load the context you know you need

Ranking the evidence you hold comes second. Acquiring it comes first. Before you
make a consequential claim, name the source that would settle it and open that
source. A claim is consequential when a reader could act on it or when it enters
a durable artifact such as an issue, plan, review, record, verdict, or
recommendation. The trigger is the claim, not an edit. An assessment, a ranking,
and a diagnosis reach it exactly as a code change does.

Prefer the thing over any description of the thing. Read the code rather than
the issue describing it. Read the diff rather than the commit subject. Read the
file contents rather than the metadata or the search hit. Read the raw response
rather than a summary of it. A description is evidence about the description. It
can be stale, partial, or backwards relative to the thing it names.

Naming a gap is not closing it. An identified gap is a task, not a disclaimer.
Recording that information is still needed and then stopping is a failure
whenever that information is reachable with the access you already hold. Absence
established through one search modality is not absence. Searching issues
establishes nothing about a repository tree, and a single empty query is not a
negative result.

Apply this stopping condition before you deliver. For every consequential claim,
either name the source you opened, or mark the claim as inference and state the
observation that would settle it. Unavailability never silently promotes a guess
into a fact.

Editing is one instance of this rule rather than the boundary of it. When a
convention, schema, or subsystem wiring is discoverable in the repo, a skill, or
a doc, read it before planning, and before the first edit list the conventions
and subsystems the work touches and confirm you have read each one. A narrowed
scope does not narrow the context budget. The **first** instance of a pattern
needs the most grounding, because that first entry sets the schema everything
after it copies.

Acquisition is bounded. It reaches only sources that would change a specific
pending claim or decision, and curiosity alone is not a warrant. Role doctrine
may narrow the reach this rule grants, and where the two disagree about which
sources are yours to open, the narrower boundary wins. Cost scales
with stakes, so a durable artifact or an external commitment earns more digging
than a passing remark. This rule grants no new authority. It adds no permission,
credential, network access, or mutation right, leaves every live-operations
boundary exactly where it stands, and keeps sending, publishing, and destructive
actions gated as they are. Read-only acquisition was already permitted. The
failure this corrects is leaving it unused.

### Command delivery

Commands for a human must cross the current execution boundary truthfully.

* **Container / surface session** - a `warded` container or read-only director surface has no writable host mount. Hand one-off commands back inline. For anything reusable or worth tracking, commit to a **pushable** repo and push, then hand back the committed path. A local container file does not cross this boundary.
* **Host harness** - hand one-off commands back inline. A temporary file is optional when it materially improves review or safety, not a terminal-specific paste requirement.

In either model a **reusable script** - anything the human might run more than once, or worth tracking - is committed to a repo and handed back as a path. This covers **any** command offered for the human to run, optional and alternative ones included, not just the primary next step. The trigger is the recipient, not the framing: commands the agent runs itself through its shell execution tool never touch a human paste path and stay out of scope.

That covers a **human** recipient. There is **no autonomous agent-to-agent command channel**. The o2r channel (the `otel-a2a-relay` relay plus its `o2r` CLI) was **archived in the June 2026 surface reduction** (`agentic-os-kai#677`), kept active but never used autonomously. Delivery is now **human-mediated**: route the request through the human, who relays it upstream, no command crossing an agent boundary on its own. Revival and absorption are tracked at `ward#104`. The `kai-command-handover` skill holds the current procedure.

### Keep FEATURES.md current

When a change adds, removes, or materially reshapes a feature, update that repo's `docs/FEATURES.md` in the same commit. It is the coarse inventory of major shipped capabilities, not a changelog or bugfix ledger. Add an entry only for a new or removed significant capability: a subsystem, command family, deploy target, major integration, or broad human-facing behavior. For an existing capability, update the entry only when the public boundary materially changes. Do not update FEATURES for bugfixes, diagnostics or error-message fixes, validation hardening, CI or build fixes, dependency bumps, refactors, docs-only changes, internal plumbing, or small behavior changes. Put those details in the specific docs page, PR or issue, release notes, or code comments. Pair a substantial feature with its own `docs/<feature>.md` walkthrough and link it from the FEATURES entry.

### Comment the surprise, not the diff

Route explanation by readership. Kai reads a `docs/` page top to bottom most times she opens one, and a given code comment close to never. Agents have been writing as if the reverse held: paragraphs of comment stapled to one-line changes, at a volume that is not thoroughness but landfill burying the one line a reader needed. Cutting comment volume is the point here, not a side effect. Send the explanation to a docs page and leave a short pointer in the code.

What survives inline is what the code cannot say - a non-obvious constraint, a rejected alternative, a reason the shape looks wrong. A comment never narrates the change that introduced it. A one-line edit takes zero or one line of comment, and restating the line above, summarizing the commit, or parking session reasoning in a block goes stale on the next edit. Match the comment density of the surrounding code rather than raising it. The `code-comments` hook enforces shape in catalog repos (caps in [docs/catalog-caps-reference.md](docs/catalog-caps-reference.md)) but cannot see change size, so this binds every repo an agent touches, hooked or not.

### No auto-memory

Do not write auto-memory files in any harness that offers them. Skip the save step entirely - no new files, no `MEMORY.md` updates, no edits to existing entries, even when the harness base prompt's own memory rule tells you to save. Point-in-time memory drifts: a fact true when written goes stale and then silently anchors a wrong picture in a later session, which is worse than no note at all. Keep within-session state in plans, tasks, and the conversation. Promote anything durable into an `AGENTS.md` edit (proposed for review), where the rule lives next to what it amends instead of in an unversioned side store. Reading existing memory is fine when a store is non-empty, but the default expectation is empty.

## See also

- [README.md](README.md) - human-facing intro, per-OS install steps.
- [docs/FEATURES.md](docs/FEATURES.md) - inventory of what ships today.
- [justfile](justfile) - dev verbs. Agents route through just, not bare tooling.
- [.ward/ward.yaml](.ward/ward.yaml) - catalog metadata only.

Cross-reference convention from [release.md](docs/release.md).
