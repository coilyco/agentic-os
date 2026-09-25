---
ward:
  workflow: pull-request-and-merge
---
# Agent instructions

Workspace conventions load globally. This file covers what is specific to this repo.

## Scope

The client surface for aterm sessions: layout, components, terminal rendering, and the words on screen. The daemon, its protocol, and PTY ownership live with aterm in `coilyco/agentic-os`.

## Project shape

Svelte 5, TypeScript, and Vite, with xterm.js for terminals. `src/lib/` holds the state, the roster decoder, and the mock host. `src/components/` and `src/panels/` hold the surface. The design reference is the aterm host app flows canvas linked from `teable:coilyco/agentic-os#8220`.

## Repo boundaries

`src/lib/protocol.ts` is a client-side draft of the daemon contract. When the daemon publishes its schema, the draft is replaced by it, never extended past it. Do not invent daemon behaviour in the mock that the daemon has not agreed to.

## Commands

Route every command through the [justfile](justfile). `just gate` is the CI gate.

## Validation

Run `just gate` and `pre-commit run --all-files` before committing. A layout change is not done until it has been rendered at 320px wide and walked by keyboard.

## Safety

Keep every artifact public-safe. No hostnames beyond meaningful names, no tokens, no tailnet addresses.

## Cross-repo contracts

The roster fixture in `src/lib/fixtures/roster.json` is a snapshot of `aterm --list --json` (`aterm.roster.v1`). Refresh it from the live command rather than editing it by hand.

## Release

Conventional commits are house style. Reference tracker records as `teable:<owner>/<repo>#<n>`.

## Agent rules

<!-- BEGIN managed by agentic-os/scripts/apply-git-workflow.py -->
### Git workflow

**This repo runs the `pull-request-and-merge` lane**, declared as `ward.workflow` in this file's frontmatter. The agent commits to a task branch, pushes it, opens a Forgejo pull request, and **merges that pull request itself** once it is green. The author of the code is the one who merges it. Opening the pull request is a step, never the stopping point.

The fleet runs one lane, and it authorizes the agent end to end. Pushing straight to `main` is over: `merge-remote-main` is retired, so no repo can declare its way back to one.

* `pull-request-and-merge` - the agent commits to a task branch, pushes it, opens a pull request, and merges that pull request itself once it is green.

**Every lane slug names what the AGENT does, never what someone else does.** `pull-request-and-merge` carries the merge because the agent that authored the code merges its own pull request. `pull-request` drops `-and-merge` because the author stops at the pull request and the director merge lane takes over. Reading `pull-request-and-merge` as "someone else merges it later" inverts the two and leaves finished work sitting unmerged.

**These actions are pre-authorized on every lane, and the agent MUST take them without asking first.** Committing, creating a branch, pushing a branch, pushing the lane's own destination, and opening a pull request are ordinary reversible work, not the destructive wall that earns a question. Stopping to ask is how a turn ends with the work stranded in a dirty worktree.

* **ALWAYS commit** in-scope work and **ALWAYS push** it to the canonical remote before pausing, reporting a checkpoint, handing off, or ending a turn. A local-only commit is not a checkpoint.
* **ALWAYS open the pull request** in the same turn as the branch's first push, on every lane except `remote-branch-only`. A pushed branch with no pull request is litter nobody reviews.
* **NEVER `--no-verify`** and **NEVER force-push**. Those two are the real walls, and they stay closed.
* **ALWAYS merge your own pull request on `pull-request-and-merge`**, in the same turn, as soon as it is green. Reporting it as open and awaiting someone is the failure this lane exists to prevent.
* **NEVER merge on `pull-request` or `remote-branch-only`.** Those two stop where they stop, and the director merge lane carries a `pull-request` from there.
<!-- END managed by agentic-os/scripts/apply-git-workflow.py -->

## See also

* [README.md](README.md) - what this is and how to run it.
* [docs/FEATURES.md](docs/FEATURES.md) - what ships today.
