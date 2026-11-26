# Keeping a personal host awake

The [aterm host daemon](../../../../docs/aterm-daemon.md) keeps the computer from idle sleep while any seat runs on a host of kind `personal`, so a desktop left alone does not suspend a seat mid-task (COI-3768). The code is `aterm/keepawake.go` and one `awake_<os>.go` per platform.

## What it does

* **When.** One OS lock is held while at least one seat is live and the host kind is `personal`. A host with no `host.json` reads as personal. `durable` and `ephemeral` hosts take none (COI-3767, `aterm host kind <k>`). Terminals are not seats and do not count.
* **Released** when the last seat ends, when the host kind leaves `personal`, when the daemon stops, and on a daemon crash, because each lock is tied to the daemon's process and not to a release call.
* **System, not display.** It holds off idle system sleep only. The screen still dims and locks on its own timer. Closing a laptop lid, pressing the power button, or choosing Sleep still sleeps the machine.
* **Visible.** `aterm status <seat>` prints `host awake: held, <what holds it>`, or `not held` with the reason, and `--json` carries it as `host_awake`. The daemon log records each take and release.
* **Off switch.** `ATERM_KEEP_AWAKE=off` in the daemon's environment takes no lock at all. The test suite sets it.

## Per platform

* **macOS** - the daemon runs `/usr/bin/caffeinate -i -w <daemon pid>`. `-i` asserts against idle system sleep only, and `-w` releases the assertion when the daemon's pid exits, per `man caffeinate`. Release is a SIGTERM to caffeinate. `pmset -g assertions` lists it as `caffeinate command-line tool`, `PreventUserIdleSystemSleep`, with `Created for PID: <daemon pid>`. Naming Treehouse there would need `IOPMAssertionCreateWithName` in process, which needs cgo, and the release build is `CGO_ENABLED=0` (`scripts/aos-release-build.sh`).
* **Linux** - the daemon runs `systemd-inhibit --what=idle:sleep --who=Treehouse --mode=block sh -c 'echo held && exec cat'`, with `cat` reading a pipe only the daemon writes. The child runs only once logind grants the lock, so `held` on its stdout confirms it. When the daemon exits or crashes, the pipe closes, `cat` ends, and logind drops the lock, since a lock lives exactly as long as its file descriptor ([inhibitor locks](https://systemd.io/INHIBITOR_LOCKS/), [systemd-inhibit](https://www.freedesktop.org/software/systemd/man/latest/systemd-inhibit.html)). systemd's polkit policy lets any process take an `idle` lock, but a `sleep` lock outside an active or inactive login session wants an admin (`org.freedesktop.login1.inhibit-block-sleep`), so a daemon running as a user service falls back to `idle` alone. `systemd-inhibit --list` shows `Treehouse`. A `sleep` lock in `block` mode also makes a hand-picked Suspend ask first on desktops that check inhibitors.
* **Windows** - not built, because aterm ships no Windows binary (`aterm/release-targets.txt`). The mechanism there would be [`SetThreadExecutionState(ES_CONTINUOUS | ES_SYSTEM_REQUIRED)`](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-setthreadexecutionstate) on one locked OS thread, cleared with `ES_CONTINUOUS` alone. Microsoft documents that it cannot stop a lid close or the power button, and that it holds while the application runs.

## Checked

* `go test -run 'KeepAwake|Caffeinate|Inhibit'` covers one lock shared by many seats, release at the last seat, no lock off a personal host and release on turning durable, a dead lock taken again, a failure reported in status, and release at daemon stop.
* `TestCaffeinateLockShowsInPmsetAndReleases` takes the real macOS lock, reads it back from `pmset -g assertions`, releases it, and reads it gone. `TestInhibitLockShowsInLogindAndReleases` does the same against `systemd-inhibit --list` where logind runs, and skips elsewhere.
