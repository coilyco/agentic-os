# The fs guard: `rm`, `find` and `mv`

`rm`, `find` and `mv` on an agent session's PATH refuse a path outside the session shadow and the Claude scratchpad. Under `--dangerously-skip-permissions` a recursive delete of a real checkout or home directory otherwise has nothing in front of it (COI-2724).

## What it checks

* **Inert without roots.** With neither `AOS_NATIVE_SESSION_PROJECTS` nor `AOS_FS_GUARD_ROOTS` set, every call passes through. The dev-base image puts the stubs on `PATH` for CI jobs as well, and a guard that refused every `rm` there would break them. The `aos` container plans, standalone and acompose, export `AOS_FS_GUARD_ROOTS` as the workspace mount plus `/tmp`, a `:` list of directories a seat may change, and leave `PATH` to the image (COI-2731, COI-2734). A container started any other way is unguarded.
* **Allowed roots** are the shadow (the parent of `AOS_NATIVE_SESSION_PROJECTS`, which holds `projects/`, `home/` and `seat/`) and `/private/tmp/claude-<uid>` or `/tmp/claude-<uid>`. A root that is `/` or the home directory is dropped, so a mis-set variable cannot widen the boundary. If every named root is dropped, every call is refused.
* **`rm`** needs every operand inside. **`mv`** needs every operand inside, source and target, because a move deletes one and overwrites the other. `-t` and `--target-directory=` count as operands.
* **`find`** is refused only when it carries `-delete`, `-exec`, `-execdir`, `-ok`, `-okdir` or an `-fprint` family predicate, and then every start path must be inside. A read-only `find` passes anywhere.
* **Path resolution** follows symlinks in the parent and leaves the leaf alone, since `rm` and `mv` act on the link. A relative path is resolved against the working directory, and `--` hides no operand.

Refusal is exit 2 with the path named. A passing call `exec`s the first matching tool after the guard's own `PATH` entry, with a `PATH` that no longer holds it. Resolving only past its own entry is what stops two installs of the guard, or a wrapper, finding each other forever.

## Temp files in a seat (COI-3177)

The guard allows only the shadow, and the macOS default `TMPDIR` under `/var/folders` is outside it, so a script's `mktemp` succeeded and its `rm` cleanup was refused. Under `set -e` the script stopped halfway. Two parts close it.

* **The launcher** exports `TMPDIR` as `<session root>/tmp`, created 0700, in a launch that links a worktree (`exportNativeSessionTemp` in `aos-cli/native_shadow.go`). An operator `TMPDIR` is replaced, because macOS always sets one, and the directory goes with the session when it is reaped. It also pins `AOS_NATIVE_SESSIONS_DIR` to the root it resolved, unless the operator set one, because other hosts derive that root from `TMPDIR` and a nested launch would resolve one inside this session.
* **A `mktemp` stub** covers what `TMPDIR` cannot. macOS `mktemp` reads `confstr(_CS_DARWIN_USER_TEMP_DIR)` for its bare and `-t` forms and ignores `TMPDIR`, while Python, perl and GNU `mktemp` follow it. The stub adds `-p $TMPDIR` only when the guard has roots, `TMPDIR` resolves inside them, and the call names no template and no `-p`. Those pass through, as does a `TMPDIR` outside the roots. `install.sh` writes it on Darwin only.

Not covered: a script naming a path under the host temp directory or `/tmp` itself, a launch that links no worktree (no markers, so the guard is inert), and a seat whose shim directory has not been converged since the stub landed. Ansible runs `install.sh` for it, and a hand rollout is `just fs-shims-install <dir>`.

## Why it is a script and not an umbra shim

umbra cannot say "inside a directory known only at runtime". A `can run "*"` funnel refuses an `argv` or `embed` override, the preflight `gate` registry ships empty, and a glob can only list paths to deny. This is the closest fit to the [guarded `git`](git-shim.md) the grammar allows, and the missing umbra primitive is COI-2728. Rollout to hosts is COI-2729.

## Building and installing

`just fs-shims-install <dir>` copies `scripts/fs-shims/fs_guard.py` into `<dir>` and writes three two-line stubs beside it, and a fourth for `mktemp` on macOS. The stubs find the guard next to themselves, so install copies and does not link. `tests/test_fs_shims.py` runs the installed stubs against a temporary shadow.

## What it does not stop

* **Shell redirection.** `> file` truncates through the shell and no binary sees it.
* **Other deleters.** `cp`, `tee`, `dd of=`, `truncate`, `shred`, `rsync --delete`, `git clean`, `git reset --hard`, and any interpreter calling `unlink` or `rmtree`.
* **The variable.** `AOS_NATIVE_SESSION_PROJECTS=/ rm ...` is a bad root and is dropped, but a caller can still point it at another real directory. The environment is the caller's.
* **A shim is not an enforcement floor.** `/bin/rm` walks around it.
