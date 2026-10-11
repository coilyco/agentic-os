"""Refuse rm, find and mv when they would touch a path outside the session shadow.

The `rm`, `find` and `mv` stubs beside this file call it as `fs_guard.py <tool>
<caller args...>`. A call whose every path sits inside the session shadow or the
Claude scratchpad runs the real tool, and anything else exits 2 naming the path.
It is a script and not an umbra shim because umbra cannot express "inside a
directory only known at runtime". Reference: tooling-aosguard, references/fs-shims.md.
"""
import os
import shutil
import sys

# find predicates that remove, rewrite or run something. Without one, find only reads.
FIND_WRITES = {
    "-delete", "-exec", "-execdir", "-ok", "-okdir",
    "-fprint", "-fprint0", "-fprintf", "-fls",
}
# mv options whose value is a path rather than the next operand's flag.
MV_PATH_FLAGS = {"-t", "--target-directory"}


def allowed_roots():
    roots = []
    projects = os.environ.get("AOS_NATIVE_SESSION_PROJECTS", "")
    if projects:
        # The shadow holds projects/, home/ and seat/ side by side.
        roots.append(os.path.dirname(os.path.realpath(projects)))
    uid = os.getuid()
    for scratch in (f"/private/tmp/claude-{uid}", f"/tmp/claude-{uid}"):
        if os.path.isdir(scratch):
            roots.append(os.path.realpath(scratch))
    home = os.path.realpath(os.path.expanduser("~"))
    # A mis-set variable must not turn "inside the shadow" into "anywhere".
    return [r for r in roots if r not in ("/", home) and r.count(os.sep) >= 3]


def resolve(path):
    """Absolute path with the parent's symlinks followed and the leaf's left alone.

    rm and mv act on a symlink, not its target, so following the leaf would
    approve a link inside the shadow that points out of it.
    """
    full = os.path.abspath(path)
    if path.endswith("/") or full == "/":
        return os.path.realpath(full)
    return os.path.join(os.path.realpath(os.path.dirname(full)), os.path.basename(full))


def inside(path, roots):
    return any(os.path.commonpath([path, root]) == root for root in roots)


def operands(tool, args):
    """The paths a call touches."""
    paths, flags_done, take_next = [], False, False
    if tool == "find":
        for arg in args:
            if arg.startswith("-") or arg in ("(", "!", ")", ","):
                break
            paths.append(arg)
        return paths or ["."]
    for arg in args:
        if take_next:
            paths.append(arg)
            take_next = False
        elif flags_done or not arg.startswith("-") or arg == "-":
            paths.append(arg)
        elif arg == "--":
            flags_done = True
        elif tool == "mv" and arg in MV_PATH_FLAGS:
            take_next = True
        elif tool == "mv" and arg.startswith("--target-directory="):
            paths.append(arg.split("=", 1)[1])
    return paths


def real_tool(tool):
    """The tool on PATH once the directory holding this guard is dropped."""
    shim_dir = os.path.dirname(os.path.realpath(__file__))
    kept = [p for p in os.environ.get("PATH", "").split(os.pathsep)
            if p and os.path.realpath(p) != shim_dir]
    return shutil.which(tool, path=os.pathsep.join(kept))


def main(argv):
    tool, args = argv[1], argv[2:]
    real = real_tool(tool)
    if real is None:
        sys.stderr.write(f"{tool}: no real {tool} on PATH\n")
        return 127
    reads_only = tool == "find" and not FIND_WRITES.intersection(args)
    paths = [] if reads_only else operands(tool, args)
    roots = allowed_roots()
    for path in paths:
        if not roots or not inside(resolve(path), roots):
            sys.stderr.write(
                f"{tool}: refused by umbra fs guard, {path!r} is outside the session shadow "
                "and the scratchpad (COI-2724). Work on a copy inside the shadow, or hand "
                "the operation to Kai.\n"
            )
            return 2
    os.execv(real, [tool, *args])


if __name__ == "__main__":
    sys.exit(main(sys.argv))
