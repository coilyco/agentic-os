"""Refuse the git spellings that skip a hook or rewrite a remote.

The git stub beside this file calls it as `git_guard.py git <args>`. It refuses
commit -n and --no-verify, push --force and its variants or a +refspec, a
--no-verify on the other hook-running verbs, and a core.hooksPath write. It is a
script because umbra's metacharacter gate refuses `<` and `(` in any argument of
a named grant, which fails every attributed commit. Reference: tooling-aosguard
references/git-shim.md.
"""
import os
import shutil
import subprocess
import sys

# Global options whose value is the next token.
GLOBAL_VALUE_OPTS = {"-C", "-c", "--git-dir", "--work-tree", "--namespace",
                     "--super-prefix", "--config-env", "--attr-source"}
HOOK_RUNNERS = {"commit", "push", "merge", "rebase", "am", "cherry-pick", "revert", "pull"}
# Long options of these verbs that take a separate value token, and the short
# options whose value is the rest of the bundle or the next token.
LONG_VALUE = {
    "commit": {"--message", "--file", "--author", "--date", "--template", "--trailer",
               "--reuse-message", "--reedit-message", "--fixup", "--squash",
               "--cleanup", "--pathspec-from-file"},
    "push": {"--push-option", "--repo", "--receive-pack", "--exec"},
}
SHORT_VALUE = {"commit": set("mFCct"), "push": set("o")}
CONFIG_READS = {"--get", "--get-all", "--get-regexp", "--get-urlmatch", "--list", "-l",
                "--unset", "--unset-all", "--show-origin", "--default"}
CONFIG_VALUE_OPTS = {"--file", "-f", "--blob", "--type", "--default"}


def abbreviates(name, full, shortest):
    """git accepts any unambiguous prefix of a long option."""
    return len(name) >= shortest and full.startswith(name)


def hits(sub, args):
    """The first refused spelling among a verb's arguments, or None."""
    long_value, short_value = LONG_VALUE.get(sub, set()), SHORT_VALUE.get(sub, set())
    i, operands, done = 0, [], False
    while i < len(args):
        arg = args[i]
        if done or arg == "-" or not arg.startswith("-"):
            operands.append(arg)
        elif arg == "--":
            done = True
        elif arg.startswith("--"):
            name = arg.split("=", 1)[0]
            if sub in HOOK_RUNNERS and abbreviates(name, "--no-verify", len("--no-v")):
                return "--no-verify"
            if sub == "push" and name.startswith("--forc"):
                return name
            if "=" not in arg and name in long_value:
                i += 1
        else:
            for pos, char in enumerate(arg[1:], 1):
                if sub == "commit" and char == "n":
                    return "-n"
                if sub == "push" and char == "f":
                    return "-f"
                if char in short_value:
                    i += pos == len(arg) - 1
                    break
        i += 1
    if sub == "push":
        plus = [o for o in operands if o.startswith("+")]
        if plus:
            return f"the forcing refspec {plus[0]}"
    return None


def config_write_of_hooks_path(args):
    """True for `git config core.hooksPath <value>`; a read or an unset is fine."""
    if CONFIG_READS.intersection(args):
        return False
    positional, skip = [], False
    for arg in args:
        if skip:
            skip = False
        elif arg in CONFIG_VALUE_OPTS:
            skip = True
        elif not arg.startswith("-"):
            positional.append(arg)
    return len(positional) >= 2 and "hookspath" in positional[0].lower()


def split_verb(argv):
    """The global options, the verb and the verb's arguments of one git call."""
    i = 0
    while i < len(argv) and argv[i].startswith("-"):
        i += 2 if argv[i] in GLOBAL_VALUE_OPTS else 1
    return argv[:i], (argv[i] if i < len(argv) else None), argv[i + 1:]


def refusal(argv):
    """The reason this git call is refused, or None."""
    for key, value in os.environ.items():
        if key.startswith("GIT_CONFIG_") and "hookspath" in value.lower():
            return f"{key} setting core.hooksPath"
        # An alias can carry --no-verify past the verb scan.
        if key == "GIT_CONFIG_PARAMETERS" and "'alias." in value.lower():
            return f"{key} defining an alias"
        if key.startswith("GIT_CONFIG_KEY_") and value.lower().startswith("alias."):
            return f"{key} defining an alias"
    options, sub, args = split_verb(argv)
    for i, opt in enumerate(options):
        if opt in ("-c", "--config-env") or opt.startswith("--config-env="):
            setting = " ".join(options[i:i + (2 if opt in GLOBAL_VALUE_OPTS else 1)]).lower()
            if "hookspath" in setting:
                return "core.hooksPath on the command line"
            if "alias." in setting:
                return "an alias defined on the command line"
    if sub is None:
        return None
    if sub == "config" and config_write_of_hooks_path(args):
        return "a write to core.hooksPath"
    found = hits(sub, args)
    return f"{found} on `{sub}`" if found else None


def hooks_path_escape(real, env, options):
    """The effective core.hooksPath when it leaves the work tree, else None.

    Asks git itself, so every source it reads counts: -c, GIT_CONFIG_*, an
    include.path, a GIT_CONFIG_GLOBAL file, and a line written straight into
    .git/config. A relative path or one inside the work tree stays, which is how
    husky works. Anything else skips the repo's hooks exactly as --no-verify does.
    """
    def ask(*argv):
        try:
            done = subprocess.run([real, *options, *argv], env=env, capture_output=True,
                                  text=True, timeout=10)
        except (OSError, subprocess.SubprocessError):
            return None
        return done.stdout.strip() if done.returncode == 0 else None

    value = ask("config", "--type=path", "--get", "core.hooksPath")
    top = ask("rev-parse", "--show-toplevel") if value else None
    if not value or not top:
        return None
    top = os.path.realpath(top)
    where = os.path.realpath(os.path.join(top, value))
    return None if os.path.commonpath([where, top]) == top else value


def real_git():
    """git and the PATH to run it with: only what follows this guard's own directory.

    A wrapper ahead of the guard has already run, and one that stayed in the child's
    PATH would run a second time, or loop, since each skips only its own directory.
    """
    shim_dir = os.path.dirname(os.path.realpath(__file__))
    parts = [p for p in os.environ.get("PATH", "").split(os.pathsep) if p]
    at = [i for i, p in enumerate(parts) if os.path.realpath(p) == shim_dir]
    rest = parts[at[0] + 1:] if at else parts
    kept = os.pathsep.join(p for p in rest if os.path.realpath(p) != shim_dir)
    return shutil.which("git", path=kept), kept


def main(argv):
    args = argv[2:]
    real, kept = real_git()
    if real is None:
        sys.stderr.write("git: no real git on PATH\n")
        return 127
    reason = refusal(args)
    child = {**os.environ, "PATH": kept}
    options, sub, _ = split_verb(args)
    if not reason and sub in HOOK_RUNNERS:
        escaped = hooks_path_escape(real, child, options)
        reason = f"an effective core.hooksPath of {escaped!r}, outside the work tree" if escaped else None
    if reason:
        sys.stderr.write(f"git: {reason} is denied (umbra git guard, COI-2724)\n")
        return 2
    os.execve(real, ["git", *args], child)


if __name__ == "__main__":
    sys.exit(main(sys.argv))
