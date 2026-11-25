#!/usr/bin/env python3
"""PreToolUse Bash hook: refuse a seat's `claude` launch that starts a login (COI-3682).

A `claude` with a clean environment or another HOME or config directory has no login,
so its login screen opens the host browser and asks the Keychain for a missing item.
`env -i` drops PATH, so a shim cannot stop it and the guard reads the command instead.
Reads the hook JSON on stdin, exits 2 with the reason on stderr to refuse, and passes
the call when it fails itself. Limits: tooling-aosguard references/login-guard.md.
"""
from __future__ import annotations

import json
import os
import re
import shlex
import sys
from pathlib import PurePosixPath

CLAUDE_NAMES = {"claude", "claude-code"}
# What a login follows. HOME moves ~/.claude, the other two name the directory itself.
LOGIN_VARS = ("HOME", "CLAUDE_CONFIG_DIR", "XDG_CONFIG_HOME")
# A launch that prints and exits never reaches a login screen.
HARMLESS_ARGS = {("--version",), ("-v",), ("--help",), ("-h",)}
# Words after which a command can be a plain wrapper, so the next word is the command.
WRAPPERS = {"command", "exec", "builtin", "nohup", "time", "nice", "caffeinate"}
# Commands that run a string or another command, so their arguments are read again.
SHELLS = {"sh", "bash", "zsh", "dash", "ksh"}
RUNNERS = SHELLS | {"eval", "tmux", "screen", "script", "su", "watch", "xargs"}
SEPARATORS = set(";&|()<>")

REASON = (
    "refused: a `claude` launched with {why} has no login. Its login screen opens the "
    "host browser and asks the login Keychain for an item that does not exist, on Kai's "
    "own screen (COI-3682). Test a login flow from captured screens, the dev Mock host, or the "
    "emulator lane, never with a real login on this host (tooling-aterm-client)."
)


def _words(command: str) -> list[str]:
    """Split a shell line into words, with every operator and newline its own word."""
    text = command.replace("`", " ; ").replace("\n", " ; ")
    try:
        lexer = shlex.shlex(text, posix=True, punctuation_chars=True)
        lexer.whitespace_split = True
        return list(lexer)
    except ValueError:
        return text.split()


def _segments(words: list[str]) -> list[list[str]]:
    """Split at operators, so each segment is one simple command."""
    segments: list[list[str]] = [[]]
    for word in words:
        if word and set(word) <= SEPARATORS:
            segments.append([])
        else:
            segments[-1].append(word)
    return [segment for segment in segments if segment]


def _assignment(word: str) -> tuple[str, str] | None:
    name, sep, value = word.partition("=")
    if sep and name.isidentifier():
        return name, value
    return None


def _expand(value: str) -> str:
    return os.path.expanduser(os.path.expandvars(value))


def _differs(name: str, value: str) -> bool:
    """True unless the assignment leaves the variable as this seat already has it."""
    return _expand(value) != os.environ.get(name, "")


def _inspect(command: str, carried: dict[str, str], found: list[str]) -> None:
    """Walk one command string, noting every refusable `claude` launch in `found`.

    carried holds `export NAME=value` lines seen earlier in the same command, since
    `export HOME=/x; claude` changes the launch without touching the claude segment.
    """
    for segment in _segments(_words(command)):
        cleared = False
        assigned: dict[str, str] = dict(carried)
        index = 0
        while index < len(segment):
            word = segment[index]
            pair = _assignment(word)
            if pair:
                assigned[pair[0]] = pair[1]
                if index == 0 and len(segment) == 1:
                    carried[pair[0]] = pair[1]
            elif word == "export" and index == 0:
                for rest in segment[1:]:
                    exported = _assignment(rest)
                    if exported:
                        carried[exported[0]] = exported[1]
                        assigned[exported[0]] = exported[1]
                index = len(segment)
                break
            elif PurePosixPath(word).name == "env":
                index, cleared = _env_prefix(segment, index, assigned, found, cleared)
                continue
            elif PurePosixPath(word).name in WRAPPERS:
                pass
            else:
                break
            index += 1
        if index >= len(segment):
            continue
        command_word = PurePosixPath(segment[index]).name
        if command_word in CLAUDE_NAMES:
            why = _why(cleared, assigned, tuple(segment[index + 1:]))
            if why:
                found.append(why)
        elif command_word in RUNNERS:
            for argument in _command_strings(command_word, segment[index + 1:]):
                _inspect(argument, dict(assigned), found)


def _command_strings(runner: str, arguments: list[str]) -> list[str]:
    """The arguments of a runner that are themselves a command line to read again."""
    if runner == "eval":
        return [" ".join(arguments)]
    if runner in SHELLS:
        # `bash -c 'cmd'`, `bash -lc 'cmd'`: the word after a flag cluster holding c.
        return [
            arguments[i + 1]
            for i, word in enumerate(arguments[:-1])
            if re.fullmatch(r"-[a-zA-Z]*c[a-zA-Z]*", word)
        ]
    # tmux, screen, script and the rest take a command as one quoted argument.
    return [argument for argument in arguments if len(argument.split()) > 1]


def _env_prefix(segment: list[str], index: int, assigned: dict[str, str], found: list[str], cleared: bool) -> tuple[int, bool]:
    """Read `env` and its options, returning the index of the command it runs."""
    index += 1
    while index < len(segment):
        word = segment[index]
        pair = _assignment(word)
        if pair:
            assigned[pair[0]] = pair[1]
        elif word in {"-", "--ignore-environment"} or re.fullmatch(r"-[0v]*i[0v]*", word):
            cleared = True
        elif word in {"-S", "--split-string"} and index + 1 < len(segment):
            _inspect(segment[index + 1], dict(assigned), found)
            index += 1
        elif word.startswith("--split-string="):
            _inspect(word.partition("=")[2], dict(assigned), found)
        elif word in {"-u", "-C", "-P", "--unset", "--chdir"}:
            index += 1
        elif word.startswith("-"):
            pass
        else:
            break
        index += 1
    return index, cleared


def _why(cleared: bool, assigned: dict[str, str], arguments: tuple[str, ...]) -> str:
    if arguments in HARMLESS_ARGS:
        return ""
    if cleared:
        return "a cleared environment (`env -i`)"
    for name in LOGIN_VARS:
        if name in assigned and _differs(name, assigned[name]):
            return f"`{name}` set to somewhere other than this seat's own"
    return ""


def refusal(command: str) -> str:
    """The reason to refuse this command, or an empty string to let it run."""
    found: list[str] = []
    _inspect(command, {}, found)
    return REASON.format(why=found[0]) if found else ""


def main() -> int:
    try:
        payload = json.load(sys.stdin)
        if payload.get("tool_name") != "Bash":
            return 0
        command = (payload.get("tool_input") or {}).get("command")
        reason = refusal(command) if isinstance(command, str) else ""
    except Exception as error:  # noqa: BLE001 - a broken guard fails open
        print(f"seat_login_guard: passed the call, it could not read it: {error}", file=sys.stderr)
        return 0
    if reason:
        print(reason, file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
