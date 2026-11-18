# The seat login guard

A `claude` with no login offers one, and picking it runs the host opener and reaches
for a Keychain item that does not exist, on Kai's own screen (COI-3682). Seats testing
a login flow ran `env -i HOME=<scratch> claude`. That is a fresh config directory,
and a fresh directory is a login nobody has done ([credentials](../../../../docs/native-claude-credentials.md)).
`env -i` also drops `PATH` and the `BROWSER` shim of
[open-url](../../tooling-aterm-client/references/open-url.md), so no shim
could catch it.

[`scripts/seat_login_guard.py`](../../../../scripts/seat_login_guard.py) is a PreToolUse Bash
hook that reads the command and refuses before the process starts. The doctrine it
backs is in the `tooling-aterm-client` skill: login flows are tested from captured
screens, demo mode, or the emulator lane.

## Rollout

`apply-base-claude-settings.py` registers the hook in `~/.claude/settings.json`,
beside the attribution hook and repointing a copy wired from another checkout. The
`claude-hooks` role in infrastructure runs it, as for the other settings guardrails.
A host is guarded once that role has converged it. Until then the doctrine is the
only layer.

## What it decides

* **Refused** - a `claude` launched with `env -i`, or with `HOME`,
  `CLAUDE_CONFIG_DIR` or `XDG_CONFIG_HOME` set to anything but the seat's own value.
  It reads through `env`, `export`, `sh -c`, `eval` and tmux command strings. Exit 2
  names the record and the safe alternatives.
* **Passes** - plain `claude`, the seats directory itself, `--version` and `--help`,
  and any command that only mentions the word.
* **Fails open** on input it cannot read, so a broken guard stops no seat.

## What it does not stop

A script file that runs `claude`, a launch that never goes through the Bash tool,
another harness's login, and a person at a plain terminal. A sandbox or a separate
seat user would stop them, and COI-3013 rules both out. Heredoc text that spells the
refused form reads as a command, so pass such text through a file.

## Tests

`tests/test_seat_login_guard.py` feeds command strings to the hook and never runs
`claude`. `tests/test_apply_base_claude_settings.py` holds the registration.
