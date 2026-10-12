# The guarded `git`

`git` on an agent session's PATH is a stub that runs [`scripts/fs-shims/git_guard.py`](../../../../scripts/fs-shims/git_guard.py) before the real git, placed the same way as the [fs guard](fs-shims.md) and ahead of the [guarded `gh`](gh-replacement.md). It exists because `--dangerously-skip-permissions` removes the harness prompt, so a refusal that lives in the binary is the layer still standing (COI-2724).

## What it refuses

Everything unnamed is the ordinary git. Refusals are scoped to the verb, so `git log -n 5` and `git clean -f` are untouched.

* **`commit -n` and `--no-verify`**, including a `-n` inside a bundle (`-an`), after other flags, and an abbreviation such as `--no-verif`. A message or author value is skipped, so `-m "fix -n"` passes.
* **`push` `-f`, any `--forc...` option** (`--force`, `--force-with-lease`, `--force-if-includes`), and a leading `+` on a refspec, which forces one ref with no flag.
* **`--no-verify` on `push`, `merge`, `rebase`, `am`, `cherry-pick`, `revert` and `pull`**, which skip their hooks. `--no-verify-signatures` is a different flag and passes.
* **`core.hooksPath`** set by `git -c`, `--config-env`, a `GIT_CONFIG_*` environment variable, or a `git config` write. A read or an unset passes, so `pre-commit install` can ask.
* **An effective `core.hooksPath` outside the work tree**, on `commit`, `push`, `merge`, `rebase`, `am`, `cherry-pick`, `revert` and `pull`. The guard asks git (`config --type=path --get core.hooksPath`), so every source counts: `-c include.path=`, a `GIT_CONFIG_GLOBAL` file, a `GIT_CONFIG_COUNT` pair, a line appended to `.git/config` (COI-2730). A relative path or one inside the work tree passes, which is how husky works. If that ask fails or times out, the call passes.
* **An alias defined from outside**, by `-c alias.*`, `--config-env=alias.*`, `GIT_CONFIG_KEY_n` or `GIT_CONFIG_PARAMETERS`, since an alias can carry `--no-verify` past the verb scan.

Refusal is exit 2 naming the spelling. It reads argv, so it holds for every remote. A guard keyed on the forge remote would pass a force-push to a local bare remote, which is what the COI-1927 run measured.

## Why a script and not an umbra shim

The first version was an umbra replacement (`deny-flag ... aliases="-n"`), and it broke every attributed commit (COI-2738). umbra's shell-metacharacter gate checks every argument of a **named grant** (`can run commit`) and refuses `` ` $ ; & | < > ( ) { } \ `` plus newline and tab. The attribution wrapper passes `--author "Claude <noreply@anthropic.com>"`, and an ordinary `-m "feat(x): y"` carries parentheses. The exec dialect has no opt-out: `allow-metacharacters` exists only for HTTP guardfiles, and only unnamed `default-allow` forwards skip the gate. The only flag-aware refusals umbra can express are named grants, and `never pass` globs match a token in any verb, which would refuse `git clean -f` and `git log -n`.

umbra pinned below v0.254.0 also ignored `aliases=`, so `commit -n` and `push -f` passed. The lock and `ARG UMBRA_VERSION` stay at v0.254.0 for the `gh` shim.

## Resolving the real git

The guard runs the first `git` after its own `PATH` entry and hands that git a `PATH` without it. A wrapper ahead of the guard, such as the attribution wrapper, has already run, so finding it again would run it twice, and two wrappers that each skip only themselves loop forever. The wrapper-then-guard and guard-then-wrapper orders both work, and `tests/test_git_shim.py` runs the real attribution wrapper in front of the guard when it is installed.

## In the dev-base image

The full image installs the stubs into `/opt/agentic-os/shims` and puts that directory first on `PATH` with an image `ENV` as the last step before `WORKDIR`. It is last because every earlier `RUN` needs the real `git`, `rm` and `mv`. `ENV` reaches every process started from the image, and the same layer proves `command -v` resolves each tool there and that `git commit --no-verify` is refused, so a build that loses the shims fails. CI jobs run in this image too (COI-2731), and run the guard the image holds, not the checkout's, so a change under `scripts/fs-shims` takes effect in CI only after the image republishes. Both `dev-base-pr` and `dev-base-publish` list that directory in their path filters for that reason.

## What it does not stop

* **A shim is not an enforcement floor.** `/usr/bin/git`, or any copy of git, walks around it.
* **Absolute paths and shell redirection** are accepted risks (COI-2730, Kai on COI-2687). `/bin/rm` and `/usr/bin/git` are not on the shim's PATH, and `>` reaches no wrapped binary, so only a container or sandbox bounds them. The `core.hooksPath` consequence of a redirected write into `.git/config` is caught above.
* **A persistent alias** already in a config file that expands to `commit` or `push` is not resolved, so `git ci -n` passes when `alias.ci` is `commit`.
* **`GIT_DIR` or `--git-dir`** pointing at a repo with no hooks.
* **Server side.** Forgejo branch protection is the only layer that holds against a remote force-push from a caller who skips the shim. Whether `main` is protected against it was not checked here.
* **Cost.** Each git call starts a Python interpreter first.
