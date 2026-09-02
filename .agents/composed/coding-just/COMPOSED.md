---
name: coding-just
description: The justfile is the only dev-command surface. Bare `just` lists the verbs, a verb must exist before it is invoked, and operator verbs are aosguard's. Triggers - just, justfile, dev command
---

# coding-just

Every dev command in Kai's repositories routes through that repo's
`justfile`. Agents invoke `just <verb> <args>` instead of bare `make`, `uv`,
`python`, `npm`, `cargo`, or `dotnet`. The host lockdown denies some bare
invocations and not others, so a bare command that happens to run is not
evidence it was the right surface.

## Triggers

just, justfile, just verb, recipe, task runner, dev command, build command,
test command, how do I run this, what commands does this repo have.

## Discovery

Bare `just` lists every verb with its comment. That is the discovery path.
Reading the justfile is the fallback when a verb's behavior is unclear, not the
first move.

A verb must exist before it is invoked. Adding one to the justfile is part of
the work instead of a follow-up, and inventing an invocation the file does not
define is the failure this convention exists to prevent.

## Conventional verbs

Common across repositories, though no repo is required to have all of
them. Their absence is information: it usually means the repo has no such
surface, not that the verb was forgotten.

- `just check` - the repo's own validation gate
- `just test` - the test suite
- `just up-to-date` - read-only drift detectors
- `just refresh-symlinks` - skill catalog wiring, in the agent-context repos

## Passthrough

Several repositories accept native flags after the verb, either directly or
after a `--` separator, depending on how the recipe captures arguments. Check
the recipe instead of assuming a form: `sync-repo-skills --verify-manifest`
and `sync-mcp-skills -- --check` both exist in the same repo.

## What the justfile does not own

Operator verbs are **aosguard's**, surfaced as `aosguard ops <area> ...`, and
the two surfaces do not overlap. Forgejo, AWS and SSM, Tailscale, and kubectl
are aosguard. Repo development commands are the justfile's. Enumerate
operator verbs with `aosguard ops <area> describe` instead of guessing a name.

## See also

- `tooling-aosguard` - the operator surface this one does not cover.
