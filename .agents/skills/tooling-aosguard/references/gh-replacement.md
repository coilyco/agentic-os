# The guarded `gh`

`gh` on an agent session's PATH is not GitHub's `gh`. It is a generated umbra replacement built from [`.umbra/guardfiles/gh/gh.kdl`](../../../../.umbra/guardfiles/gh/gh.kdl), and it refuses every pull-request write with the reason Forgejo owns them. The real binary is still there, one absolute path away, and every other verb reaches it untouched.

## Why the guard is the binary rather than a harness rule

The first attempt put this in a Claude Code `PreToolUse` hook with settings deny rules under it, and that was reverted (agentic-os#1594). A harness hook binds one harness: Codex, a bare shell, a script, and a subagent all walked past it, and the refusal it produced was a per-harness reimplementation of a boundary the guardfile already states. A replacement binds the name instead, so whoever types `gh` meets it.

## The file is a boundary, not a catalog

The wrap declares `default-allow`, so an unnamed verb forwards to the real `gh`. **Naming a verb here is how something is taken away, not how it is added.** Two things are named:

* **Eight `withhold` stubs** over the pull-request writes: `create`, `merge`, `edit`, `close`, `reopen`, `ready`, `review`, `comment`. Each mounts a real leaf whose `--help` line leads with `NOT AVAILABLE - withheld by policy`, and whose invocation exits `policy_denied` (2) carrying its reason, which names the `aosguard ops forgejo pr` verb that replaces it.
* **`api`, allowlisted down to a GET.** `gh api -f title=x <path>` flips to POST with no method flag anywhere in the argv, so denying `--method` and `-X` would pass that spelling untouched. Only an allowlist sees the shape. The two real fleet callers pass `--cache` and `--jq` and nothing else.

The first version of this file enumerated 23 read grants to protect those 8 withholds. That list was an inventory of GitHub's CLI rather than a statement of this estate's policy, so it would have rotted against `gh` releases, and a verb nobody thought of would have been a verb the fleet silently lost. `default-allow` was added to umbra for exactly this case (umbra#372).

`withhold` rather than `never run`, and the difference is the whole design. An exec-dialect `never run` mounts nothing at all, so the caller gets `unknown verb`, which reads as a wrapper missing a feature. Under default-allow it is worse than that: an unnamed verb forwards, so silence would hand the caller the very thing this refuses.

`gh issue create` forwards. The GitHub issue queue is for external contributors and fleet work goes to Teable, but three sources disagree about whether an agent may ever file one, and that is open at `teable:coilyco-flight-deck/agentic-os#7380` rather than decided by this guardfile.

## Where it sits, and what that does not buy

`shell/common.sh` prepends `~/.local/umbra/shims` only when `AOS_NATIVE_SESSION` is set, so an agent session gets the guarded `gh` and an ordinary interactive shell keeps the real one. Kai's call: the refusal is for agents, and a human who wants `gh pr create` should not have to fight her own machine for it.

umbra's own doc is blunt about the limit, and it is worth restating here: **a PATH shim is not an enforcement floor.** A same-user agent that spells `/opt/homebrew/bin/gh` walks around it, and nothing in umbra contains a caller who declines to be occluded. The floor would be ownership plus no passwordless sudo, which belongs to host convergence. What the replacement does buy is that the boundary is validated and audited wherever it is met, and the caller needs to know nothing about umbra to get the refusal.

`UMBRA_IDENTIFY=1 gh` is the only surface that says what stands on the name. A pre-verb flag such as `gh --version` forwards, because under default-allow it is part of the unnamed surface.

## Building and installing it

`just gh-shim-build` writes `dist/shims/gh`. The project root holds two wrap binaries, `aosguard` and `gh`, so every umbra verb against it names a member with `--guardfile`; that path is resolved against the caller's working directory rather than the project root, so the build verbs pass an absolute one.

Both members share one `specverb.lock`. `withhold` landed in umbra v0.202.0, `replace` in v0.212.0, and `default-allow` in the release this lock now pins. The dev-base `ARG UMBRA_VERSION` moves with it, enforced by `test_umbra_pin_is_owned_by_the_dependency_lock`.

Rollout is the `agentic-os-config` ansible role in `coilyco-bridge/infrastructure`, which runs `umbra install` into the shim directory and reports changed on a sha256 either side of it.
