# The guarded `gh`

`gh` on an agent session's PATH is not GitHub's `gh`. It is a generated umbra replacement built from [`.umbra/guardfiles/gh/gh.kdl`](../../../../.umbra/guardfiles/gh/gh.kdl), and it refuses every pull-request write with the reason Forgejo owns them. The real binary is still there, one absolute path away, and still reached by every granted verb.

## Why the guard is the binary rather than a harness rule

The first attempt put this in a Claude Code `PreToolUse` hook with settings deny rules under it, and that was reverted (agentic-os#1594). A harness hook binds one harness: Codex, a bare shell, a script, and a subagent all walked past it, and the refusal it produced was a per-harness reimplementation of a boundary the guardfile already states. A replacement binds the name instead, so whoever types `gh` meets it.

## What it refuses, and how that reads

Eight `withhold` stubs: `pr create`, `merge`, `edit`, `close`, `reopen`, `ready`, `review`, `comment`. Each mounts a real leaf whose `--help` line leads with `NOT AVAILABLE - withheld by policy`, and whose invocation exits `policy_denied` (2) carrying its reason, which names the `aosguard ops forgejo pr` verb that replaces it.

`withhold` rather than `never run`, and the difference is the whole design. An exec-dialect `never run` mounts nothing at all, so the caller gets `unknown verb`, which reads as a wrapper missing a feature and earns a retry or a fall back to the real binary. A stated refusal cannot be mistaken for an absence.

An **ungranted** verb (`gh workflow run`, say) is refused differently: under occlusion the wrapper cannot tell a denied verb from a misspelt one, so it says only that the name is not granted here and points at `--help`. That is umbra's behavior, not this guardfile's.

## What it grants

The verbs the fleet actually uses, measured across every checkout under `~/projects` rather than guessed: the `pr` read side, `repo` view/list/clone, `run` list/view/watch, `release` list/view/download, the `issue` verbs, `secret set`/`list`, `search code`, `auth status`/`token`, and `api`.

`api` is the one leaf carrying a flag policy, and it is an allowlist rather than a deny list. `gh api -f title=x <path>` flips to POST with no method flag anywhere in the argv, so denying `--method` and `-X` would pass that spelling untouched. Only an allowlist sees the shape. The two real fleet callers pass `--cache` and `--jq` and nothing else.

`gh issue create` is granted. The GitHub issue queue is for external contributors and fleet work goes to Teable, but three sources disagree about whether an agent may ever file one, and that is open at `teable:coilyco-flight-deck/agentic-os#7380` rather than decided by this guardfile.

## Where it sits, and what that does not buy

`shell/common.sh` prepends `~/.local/umbra/shims` only when `AOS_NATIVE_SESSION` is set, so an agent session gets the guarded `gh` and an ordinary interactive shell keeps the real one. Kai's call: the refusal is for agents, and a human who wants `gh pr create` should not have to fight her own machine for it.

umbra's own doc is blunt about the limit, and it is worth restating here: **a PATH shim is not an enforcement floor.** A same-user agent that spells `/opt/homebrew/bin/gh` walks around it, and nothing in umbra contains a caller who declines to be occluded. The floor would be ownership plus no passwordless sudo, which belongs to host convergence. What the replacement does buy is that every granted call is validated and audited, the ungranted surface is invisible rather than merely refused, and the caller needs to know nothing about umbra to get both.

Two stated limits come with it. A flag before the verb is refused, `gh --version` included, because a replacement occupies a name whose whole flag namespace belongs to the tool (umbra#7324). And `UMBRA_IDENTIFY=1 gh` is the only surface that says what stands on the name.

## Building and installing it

`just gh-shim-build` writes `dist/shims/gh`. The project root now holds two wrap binaries, `aosguard` and `gh`, so every umbra verb against it names a member with `--guardfile`; that path is resolved against the caller's working directory rather than the project root, so the build verbs pass an absolute one.

Both members share one `specverb.lock`, which pins umbra v0.218.0 because `withhold` landed in v0.202.0 and `replace` in v0.212.0. The dev-base `ARG UMBRA_VERSION` moves with it, enforced by `test_umbra_pin_is_owned_by_the_dependency_lock`.
