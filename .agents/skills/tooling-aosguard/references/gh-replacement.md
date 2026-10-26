# The guarded `gh`

`gh` on an agent session's PATH is not GitHub's `gh`. It is a generated umbra replacement built from [`.umbra/shims/gh/gh.kdl`](../../../../.umbra/shims/gh/gh.kdl), and it refuses pull-request writes except on the repos listed as GitHub-canonical, and everywhere else says Forgejo owns them. The real binary is still there, one absolute path away, and every other verb reaches it untouched.

## Why the guard is the binary rather than a harness rule

The first attempt put this in a Claude Code `PreToolUse` hook with settings deny rules under it, and that was reverted (agentic-os#1594). A harness hook binds one harness: Codex, a bare shell, a script, and a subagent all walked past it, and the refusal it produced was a per-harness reimplementation of a boundary the guardfile already states. A replacement binds the name instead, so whoever types `gh` meets it.

## The file is a boundary, not a catalog

The wrap declares `default-allow`, so an unnamed verb forwards to the real `gh`. **Naming a verb here is how something is taken away, not how it is added.** Three things are named:

* **One `withhold` stub**, `pr review`, withheld for every repo. It mounts a real leaf whose `--help` line leads with `NOT AVAILABLE - withheld by policy`, and whose invocation exits `policy_denied` (2) carrying its reason.
* **`api`, allowlisted down to a GET.** `gh api -f title=x <path>` flips to POST with no method flag anywhere in the argv, so denying `--method` and `-X` would pass that spelling untouched. Only an allowlist sees the shape. The two real fleet callers pass `--cache` and `--jq` and nothing else.
* **Per-repo grants** for `pr create`, `edit`, `merge`, `close`, `reopen`, `comment`, `ready` and `actions-permissions`, described below. `pr view` and `checks` are unnamed, so they forward for any repo.

## GitHub-canonical repos (COI-2975)

A PR merged on GitHub for a repo still Forgejo-canonical makes the next 10-minute mirror push a non-fast-forward, which only a force-push repairs. So the PR writes are granted per repo, never globally. Each is `can run` with `when repo matches <repo...>`, and a call outside the list exits 2 with `did not match any allowed pattern`.

* **A repo joins** by being added to all eight lists in `gh.kdl` in the same change that flips its mirror direction. The tests read the list from the guard's own refusal text, and fail if the lists differ or the `actions-permissions` endpoints are not the same repos.
* **`--repo OWNER/REPO` is required.** The guard reads the flag, and a repo inferred from the cwd is invisible to it.
* **`-R` is refused.** gh binds `-R` and `--repo` to one variable and the last wins, so the short spelling would retarget past the guard.
* **A URL selector is refused** on every verb that takes one (all but `create`), because gh lets the URL override `--repo`. Pass a number or a branch. A short flag's value that holds `://` is refused too, so use `--body` rather than `-b` for text with a link.
* **`merge` is squash only.** `--merge`, `-m`, `--rebase`, `-r` and `--admin` are denied.
* **`actions-permissions <endpoint>` is `gh api --method PUT <endpoint>`**, with the endpoint pinned to `repos/<repo>/actions/permissions` and `--field` and `--raw-field` the only flags. It exists as its own leaf because `api` cannot allow a write flag and pin one endpoint at once. `-X` on `api` stays refused.
* **Open: a repeated `--repo`** is read by its first occurrence and gh uses the last (COI-2977, an umbra fix). It takes typing the flag twice on purpose, and `test_a_repeated_repo_flag_is_refused` is an `xfail` until it lands.
* **At Forgejo teardown** (COI-2931) the lists and the withhold go, and AGENTS.md's lane text names GitHub.

The first version of this file enumerated 23 read grants to protect the withholds. That list was an inventory of GitHub's CLI rather than a statement of this estate's policy, so it would have rotted against `gh` releases, and a verb nobody thought of would have been a verb the fleet silently lost. `default-allow` was added to umbra for exactly this case (umbra#372).

`withhold` rather than `never run`, and the difference is the whole design. An exec-dialect `never run` mounts nothing at all, so the caller gets `unknown verb`, which reads as a wrapper missing a feature. Under default-allow it is worse than that: an unnamed verb forwards, so silence would hand the caller the very thing this refuses.

`gh issue create` forwards. The GitHub issue queue is for external contributors and fleet work goes to Linear, but three sources disagree about whether an agent may ever file one, and that is open at `teable:coilyco-flight-deck/agentic-os#7380` rather than decided by this guardfile.

## Where it sits, and what that does not buy

`shell/common.sh` prepends `~/.local/umbra/shims` only when `AOS_NATIVE_SESSION` is set, so an agent session gets the guarded `gh` and an ordinary interactive shell keeps the real one. Kai's call: the refusal is for agents, and a human who wants `gh pr create` should not have to fight her own machine for it.

umbra's own doc is blunt about the limit, and it is worth restating here: **a PATH shim is not an enforcement floor.** A same-user agent that spells `/opt/homebrew/bin/gh` walks around it, and nothing in umbra contains a caller who declines to be occluded. The floor would be ownership plus no passwordless sudo, which belongs to host convergence. What the replacement does buy is that the boundary is validated and audited wherever it is met, and the caller needs to know nothing about umbra to get the refusal.

`UMBRA_IDENTIFY=1 gh` is the only surface that says what stands on the name. A pre-verb flag such as `gh --version` forwards, because under default-allow it is part of the unnamed surface.

## Building and installing it

`just gh-shim-build` writes `dist/shims/gh`. The [guarded `git`](git-shim.md) is a script now, so `.umbra/shims` holds `gh` alone again. The rollout role in `coilyco/infrastructure` still names `.umbra/guardfiles/gh/gh.kdl`, a path that no longer exists here, and has to follow.

**`gh` has its own project root, `.umbra/shims`, and that is load-bearing.** `--project-root` is a recursive discovery root and umbra refuses to guess between members, so while `gh` sat beside `aosguard` under `.umbra/guardfiles` every verb against that root had to name one with `--guardfile`. A call site that missed the sweep failed with `2 binaries in .../guardfiles (aosguard, gh); pass --guardfile to pick one`, and the one that missed it was the release build, which is why aos published no assets between `aos-v0.322.0` and `aos-v0.329.0`. One binary per root means no call site has to name anything.

Both roots build against one committed `specverb.lock`, the copy under `guardfiles/`. It holds no per-member content at all, only `go`, `cliGuard`, `goMod` and `goSum`, so a second committed copy would be one pin in two places waiting to drift. umbra reads the lock from the project root and rejects a symlink that escapes it, so `just gh-shim-build` stages a copy into `.umbra/shims` and `.gitignore` keeps it out of the tree. `just aosguard-lock` writes the original and nothing else.

`withhold` landed in umbra v0.202.0, `replace` in v0.212.0, and `default-allow` in the release this lock now pins. The dev-base `ARG UMBRA_VERSION` moves with it, enforced by `test_umbra_pin_is_owned_by_the_dependency_lock`.

Rollout is the `agentic-os-config` ansible role in `coilyco/infrastructure`, which runs `umbra install` into the shim directory and reports changed on a sha256 either side of it.
