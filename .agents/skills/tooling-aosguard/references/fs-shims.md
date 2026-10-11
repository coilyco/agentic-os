# The fs guard: `rm`, `find` and `mv`

`rm`, `find` and `mv` on an agent session's PATH refuse a path outside the session shadow and the Claude scratchpad. Under `--dangerously-skip-permissions` a recursive delete of a real checkout or home directory otherwise has nothing in front of it (COI-2724).

## What it checks

* **Inert without roots.** With neither `AOS_NATIVE_SESSION_PROJECTS` nor `AOS_FS_GUARD_ROOTS` set, every call passes through. The dev-base image puts the stubs on `PATH` for CI jobs as well, and a guard that refused every `rm` there would break them. A container seat is therefore unguarded until its launcher exports `AOS_FS_GUARD_ROOTS`, a `:` list of directories it may change (COI-2731).
* **Allowed roots** are the shadow (the parent of `AOS_NATIVE_SESSION_PROJECTS`, which holds `projects/`, `home/` and `seat/`) and `/private/tmp/claude-<uid>` or `/tmp/claude-<uid>`. A root that is `/` or the home directory is dropped, so a mis-set variable cannot widen the boundary. If every named root is dropped, every call is refused.
* **`rm`** needs every operand inside. **`mv`** needs every operand inside, source and target, because a move deletes one and overwrites the other. `-t` and `--target-directory=` count as operands.
* **`find`** is refused only when it carries `-delete`, `-exec`, `-execdir`, `-ok`, `-okdir` or an `-fprint` family predicate, and then every start path must be inside. A read-only `find` passes anywhere.
* **Path resolution** follows symlinks in the parent and leaves the leaf alone, since `rm` and `mv` act on the link. A relative path is resolved against the working directory, and `--` hides no operand.

Refusal is exit 2 with the path named. A passing call `exec`s the real tool, found on PATH with the shim directory removed.

## Why it is a script and not an umbra shim

umbra cannot say "inside a directory known only at runtime". A `can run "*"` funnel refuses an `argv` or `embed` override, the preflight `gate` registry ships empty, and a glob can only list paths to deny. This is the closest fit to the [guarded `git`](git-shim.md) the grammar allows, and the missing umbra primitive is COI-2728. Rollout to hosts is COI-2729.

## Building and installing

`just fs-shims-install <dir>` copies `scripts/fs-shims/fs_guard.py` into `<dir>` and writes three two-line stubs beside it. The stubs find the guard next to themselves, so install copies and does not link. `tests/test_fs_shims.py` runs the installed stubs against a temporary shadow.

## What it does not stop

* **Shell redirection.** `> file` truncates through the shell and no binary sees it.
* **Other deleters.** `cp`, `tee`, `dd of=`, `truncate`, `shred`, `rsync --delete`, `git clean`, `git reset --hard`, and any interpreter calling `unlink` or `rmtree`.
* **The variable.** `AOS_NATIVE_SESSION_PROJECTS=/ rm ...` is a bad root and is dropped, but a caller can still point it at another real directory. The environment is the caller's.
* **A shim is not an enforcement floor.** `/bin/rm` walks around it.
