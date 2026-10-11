# The guarded `git`

`git` on an agent session's PATH is an umbra replacement built from [`.umbra/shims/git/git.kdl`](../../../../.umbra/shims/git/git.kdl), the sibling of the [guarded `gh`](gh-replacement.md) and placed the same way. It exists because `--dangerously-skip-permissions` removes the harness prompt, so a refusal that lives in the binary is the layer still standing (COI-2724).

## What it refuses

It declares `default-allow`, so every unnamed verb is the ordinary git. Four things are taken away:

* **`commit --no-verify` and `-n`**, plus a `-n` inside a bundle (`-an`) and a cut-short `--no-verif`.
* **`push --force`, `-f`, `--force-with-lease`, `--force-if-includes`**, and a leading `+` on any argument (`push origin +main` forces one ref with no flag).
* **`push --no-verify` and `merge --no-verify`**, which skip the pre-push and pre-merge-commit hooks.
* **`core.hooksPath`**, by any argv token containing it, which covers `git -c core.hooksPath=... commit` and `git config core.hooksPath`. Pointing it elsewhere skips every hook exactly as `--no-verify` does.

The refusal holds for every remote, because it reads argv and not the remote URL. A guard keyed on the forge remote would pass a force-push to a local bare remote, which is what the COI-1927 run measured.

## The umbra pin is load-bearing

`deny-flag ... aliases="-n"` only binds on umbra v0.254.0 or later. Before it, the property parsed and was ignored, so `commit -n` and `push -f` walked past a guardfile that looked correct. `.umbra/guardfiles/specverb.lock` and `ARG UMBRA_VERSION` in the dev-base Dockerfile are pinned to v0.254.0 for that reason, and the library the binary compiles against comes from the lock while the generated source comes from the installed driver. A host whose `umbra` is older than the lock fails the build on a module path it cannot resolve, so upgrade the driver rather than the lock.

## Building and installing

`just shims-build` writes `dist/shims/gh` and `dist/shims/git`. Both live under one project root, so every umbra call names its member with `--guardfile <absolute path>`, and a call that omits it fails with `2 binaries ... pass --guardfile`. `just guard-controls` checks the `never` and `withhold` rules. It does not count `never pass`, so the argv denials here are exercised by running the built binary.

## What it does not stop

* **A shim is not an enforcement floor.** `/usr/bin/git`, or any copy of git, walks around it, and nothing contains a same-user caller who declines to be occluded.
* **A pre-verb flag forwards** under `default-allow`, so `git -c` with a config key other than `core.hooksPath` that disables a hook is not caught.
* **`GIT_CONFIG_*` environment variables** carry the same setting without argv.
* **Server side.** Forgejo branch protection is the only layer that holds against a remote force-push from a caller who skips the shim. Whether `main` is protected against it was not checked here.
