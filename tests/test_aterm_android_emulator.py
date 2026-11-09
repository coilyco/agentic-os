"""Exercise the aterm Android emulator verb against a fake adb whose device is a directory."""

from __future__ import annotations

import os
import socket
import stat
import subprocess
import sys
import time
from pathlib import Path

import pytest

VERB = Path(__file__).resolve().parent.parent / "scripts" / "aterm_android_emulator.py"

# The fake device keeps /data/local/tmp in a scratch directory and runs shell commands
# against it, so mkdir, touch and cat behave as on a device. It logs every adb argv.
FAKE_ADB = r"""#!/bin/sh
dev="$FAKE_DEVICE"
printf '%s\n' "$*" >>"$dev/adb.log"
while [ $# -gt 0 ]; do
  case "$1" in -H | -P | -s) shift 2 ;; *) break ;; esac
done
cmd=$1
shift
case "$cmd" in
  devices) printf 'List of devices attached\n%s\tdevice\n' "${FAKE_SERIAL:-emulator-5554}" ;;
  uninstall) touch "$dev/uninstalled"; echo Success ;;
  install)
    if [ "${FAKE_INCOMPATIBLE:-0}" = 1 ] && [ ! -e "$dev/uninstalled" ]; then
      echo "Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package dev.coilyco.treehouse]" >&2
      exit 1
    fi
    echo Success ;;
  connect | forward | reverse) echo ok ;;
  shell)
    case "$1" in
      "getprop sys.boot_completed") [ "${FAKE_BOOTED:-1}" = 1 ] && echo 1 ;;
      "dumpsys input_method"*) echo "mInputShown=${FAKE_KEYBOARD:-false}" ;;
      "pidof"*) echo 4242 ;;
      "cat /proc/net/unix") echo "${FAKE_SOCKETS:-}" ;;
      *) PATH="$dev/bin:$PATH" sh -c "$(printf '%s' "$1" | sed "s#/data/local/tmp#$dev/tmp#g")" ;;
    esac ;;
esac
"""

# toybox stat takes -c %Y, BSD stat takes -f %m.
FAKE_STAT = """#!/bin/sh
if [ "$1" = "-c" ] && [ "$(uname)" = "Darwin" ]; then exec /usr/bin/stat -f %m "$3"; fi
exec /usr/bin/stat "$@"
"""


FAKE_INPUT = """#!/bin/sh
echo "input $*" >>"$FAKE_DEVICE/input.log"
"""


@pytest.fixture
def device(tmp_path: Path) -> Path:
    (tmp_path / "tmp").mkdir()
    (tmp_path / "bin").mkdir()
    for path, body in ((tmp_path / "adb", FAKE_ADB), (tmp_path / "bin" / "stat", FAKE_STAT), (tmp_path / "bin" / "input", FAKE_INPUT)):
        path.write_text(body, encoding="utf-8")
        path.chmod(path.stat().st_mode | stat.S_IXUSR)
    return tmp_path


def verb(device: Path, *args: str, holder: str = "seat-a", timeout: int = 60, **env: str) -> subprocess.CompletedProcess[str]:
    base = {
        "PATH": os.environ["PATH"],
        "HOME": str(device),
        "FAKE_DEVICE": str(device),
        "TREEHOUSE_EMULATOR_ADB": str(device / "adb"),
        "TREEHOUSE_EMULATOR_HOLDER": holder,
    }
    return subprocess.run([sys.executable, str(VERB), *args], env=base | env, capture_output=True, text=True, check=False, timeout=timeout)


def test_a_second_seat_is_refused_and_told_who_holds_it(device: Path) -> None:
    assert verb(device, "acquire").returncode == 0
    other = verb(device, "acquire", holder="seat-b")
    assert other.returncode == 6
    assert "held by seat-a" in other.stderr


def test_the_holder_reacquires_releases_and_the_next_seat_takes_it(device: Path) -> None:
    assert "acquired by seat-a" in verb(device, "acquire").stdout
    assert "already held by seat-a" in verb(device, "acquire").stdout
    assert verb(device, "release").stdout.strip() == "released"
    assert "acquired by seat-b" in verb(device, "acquire", holder="seat-b").stdout


def test_another_seat_cannot_release_without_force(device: Path) -> None:
    verb(device, "acquire")
    assert verb(device, "release", holder="seat-b").returncode == 6
    assert verb(device, "release", "--force", holder="seat-b").stdout.strip() == "released"


def test_an_idle_lock_past_the_limit_is_taken_over(device: Path) -> None:
    verb(device, "acquire")
    old = time.time() - 120
    os.utime(device / "tmp" / "treehouse-emulator.lock", (old, old))
    taken = verb(device, "acquire", holder="seat-b", TREEHOUSE_EMULATOR_LOCK_TTL="60")
    assert "taking the lock over from seat-a" in taken.stdout
    assert "acquired by seat-b" in taken.stdout


def test_driving_needs_the_lock_and_names_who_has_it(device: Path) -> None:
    free = verb(device, "tap", "10", "20")
    assert free.returncode == 6 and "not held" in free.stderr
    verb(device, "acquire")
    held = verb(device, "tap", "10", "20", holder="seat-b")
    assert held.returncode == 6 and "held by seat-a" in held.stderr
    assert verb(device, "tap", "10", "20").returncode == 0
    assert (device / "input.log").read_text(encoding="utf-8").strip() == "input tap 10 20"


def test_a_holder_is_required(device: Path) -> None:
    done = verb(device, "acquire", holder="")
    assert done.returncode == 2 and "TREEHOUSE_EMULATOR_HOLDER" in done.stderr


def test_the_address_is_a_setting(device: Path) -> None:
    verb(device, "acquire", TREEHOUSE_EMULATOR_ADB_HOST="10.1.2.3", TREEHOUSE_EMULATOR_ADB_PORT="6000", TREEHOUSE_EMULATOR_SERIAL="box:1")
    assert "-H 10.1.2.3 -P 6000 -s box:1 shell" in (device / "adb.log").read_text(encoding="utf-8")


def test_boot_is_refused_while_the_host_is_over_the_pressure_limits(device: Path) -> None:
    done = verb(device, "up", FAKE_BOOTED="0", TREEHOUSE_EMULATOR_MAX_SWAP_PCT="-1")
    assert done.returncode == 7
    assert "not booting while the host is under pressure" in done.stderr
    assert "emulator start" not in (device / "adb.log").read_text(encoding="utf-8")


def test_an_emulator_already_up_is_left_alone(device: Path) -> None:
    assert verb(device, "up").stdout.startswith("already up")


def test_keyboard_state_reads_the_input_method(device: Path) -> None:
    verb(device, "acquire")
    assert verb(device, "keyboard", "state", FAKE_KEYBOARD="true").stdout.strip() == "keyboard shown"
    assert verb(device, "keyboard", "state", FAKE_KEYBOARD="false").stdout.strip() == "keyboard hidden"


def test_a_build_with_no_devtools_socket_says_to_install_the_debug_apk(device: Path) -> None:
    verb(device, "acquire")
    done = verb(device, "devtools", FAKE_SOCKETS="@chrome_devtools_remote")
    assert done.returncode == 8
    assert "release build has no WebView DevTools" in done.stderr


# A fake ssh that, like ssh -f, leaves a child holding its pipes, and opens the port.
FAKE_SSH = """#!/bin/sh
python3 -c "
import socket, time
s = socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('127.0.0.1', $FAKE_PORT)); s.listen(5); time.sleep(20)
" &
echo $! > "$FAKE_DEVICE/ssh.pid"
exit 0
"""


def test_a_tunnel_started_with_ssh_f_does_not_hang_the_verb(device: Path) -> None:
    probe = socket.socket()
    probe.bind(("127.0.0.1", 0))
    port = probe.getsockname()[1]
    probe.close()
    ssh = device / "bin" / "ssh"
    ssh.write_text(FAKE_SSH, encoding="utf-8")
    ssh.chmod(ssh.stat().st_mode | stat.S_IXUSR)
    try:
        done = verb(
            device, "tunnel", timeout=10, PATH=f"{device / 'bin'}:{os.environ['PATH']}", FAKE_PORT=str(port),
            TREEHOUSE_EMULATOR_SSH="user@host", TREEHOUSE_EMULATOR_ADB_PORT=str(port),
        )
    finally:
        pid = (device / "ssh.pid").read_text(encoding="utf-8").strip() if (device / "ssh.pid").exists() else ""
        if pid:
            subprocess.run(["kill", pid], check=False)
    assert done.returncode == 0, done.stderr
    assert "tunnel to user@host ready" in done.stdout


def test_an_install_over_another_signing_key_says_to_reinstall(device: Path, tmp_path: Path) -> None:
    apk = tmp_path / "a.apk"
    apk.write_bytes(b"apk")
    verb(device, "acquire")
    refused = verb(device, "install", str(apk), FAKE_INCOMPATIBLE="1")
    assert refused.returncode == 1 and "install --reinstall" in refused.stderr
    assert verb(device, "install", str(apk), "--reinstall", FAKE_INCOMPATIBLE="1").stdout.startswith("installed")
