"""The fixture seat script against a throwaway aterm daemon, and its refusal of the real one."""

from __future__ import annotations

import importlib.util
import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "aterm_fixture_seats.py"

def _load():
    spec = importlib.util.spec_from_file_location("fixture_seats", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    sys.modules["fixture_seats"] = module
    spec.loader.exec_module(module)
    return module


fixtures = _load()
needs_aterm = pytest.mark.skipif(shutil.which("aterm") is None, reason="needs the aterm binary on PATH")


def script(*args: str, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run([sys.executable, str(SCRIPT), *args], env=env or os.environ, capture_output=True, text=True, check=False)


def test_the_default_daemon_socket_is_refused() -> None:
    env = {k: v for k, v in os.environ.items() if k != "ATERM_DAEMON_SOCKET"}
    done = script("up", "--socket", f"/tmp/aterm-{os.getuid()}/daemon.sock", env=env)
    assert done.returncode == 2 and "holds real seats" in done.stderr


def test_a_daemon_must_be_named() -> None:
    env = {k: v for k, v in os.environ.items() if k != "ATERM_DAEMON_SOCKET"}
    done = script("up", env=env)
    assert done.returncode == 2 and "--socket" in done.stderr


@pytest.fixture
def daemon():
    # A unix socket path is limited to about 100 bytes, so pytest's tmp_path is too long.
    root = Path(tempfile.mkdtemp(prefix="fx", dir="/tmp"))
    root.chmod(0o700)
    path = root / "d.sock"
    env = os.environ | {"ATERM_DAEMON_SOCKET": str(path), "ATERM_DAEMON_WS": "127.0.0.1:0"}
    proc = subprocess.Popen(["aterm", "daemon", "--tailnet-port", ""], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(50):
        if path.exists():
            break
        time.sleep(0.2)
    yield str(path), env
    if proc.poll() is None:
        script("down", "--socket", str(path), env=env)
    proc.terminate()
    proc.wait(timeout=10)
    leaked = _holds_left(root)
    _kill_tree(leaked)
    shutil.rmtree(root, ignore_errors=True)
    assert not leaked, f"{len(leaked)} aterm holds outlived the test daemon (COI-3636), now killed"


def _processes() -> dict[int, tuple[int, str]]:
    """pid -> (ppid, command) for every process, from one ps read."""
    shown = subprocess.run(["ps", "-axo", "pid=,ppid=,command="], capture_output=True, text=True, check=False).stdout
    table = {}
    for line in shown.splitlines():
        pid, ppid, command = line.split(None, 2)
        table[int(pid)] = (int(ppid), command)
    return table


def _holds_left(root: Path, settle: float = 5.0) -> list[int]:
    """Holds under root, after giving a seat that was just closed time to exit."""
    deadline = time.monotonic() + settle
    while True:
        found = [pid for pid, (_, cmd) in _processes().items() if "aterm hold --socket " + str(root) + "/" in cmd]
        if not found or time.monotonic() >= deadline:
            return found
        time.sleep(0.25)


def _kill_tree(roots: list[int]) -> None:
    table = _processes()
    todo, doomed = list(roots), []
    while todo:
        pid = todo.pop()
        doomed.append(pid)
        todo += [child for child, (parent, _) in table.items() if parent == pid]
    for pid in reversed(doomed):
        try:
            os.kill(pid, 9)
        except ProcessLookupError:
            pass


class Watcher:
    """A client on the daemon's unix socket that keeps what the sessions channel pushes."""

    def __init__(self, path: str):
        self.sock = socket.socket(socket.AF_UNIX)
        self.sock.settimeout(40)
        self.sock.connect(path)
        self.wire = self.sock.makefile("rw", buffering=1)
        self.send({"type": "hello", "format": "aterm.daemon.v1", "version": "test"})
        self.welcome = json.loads(self.wire.readline())
        self.send({"type": "subscribe", "id": "s", "channel": "sessions"})

    def send(self, frame: dict) -> None:
        self.wire.write(json.dumps(frame) + "\n")

    def frames(self):
        while True:
            line = self.wire.readline()
            if not line:
                return
            yield json.loads(line)


@needs_aterm
def test_up_raises_a_stamped_card_and_down_removes_the_seats(daemon) -> None:
    path, env = daemon
    count = len(fixtures.SEATS)
    assert script("up", "--socket", path, env=env).stdout.count("spawned") == count
    assert script("up", "--socket", path, env=env).stdout.count("already up") == count
    ask = next(f["ask"] for f in Watcher(path).frames() if f.get("type") == "ask" and f["ask"]["from"] == "fixture Cards")
    assert [o["label"] for o in ask["options"]] == ["red", "green", "blue"]
    assert "closed" in script("down", "--socket", path, env=env).stdout
    assert script("status", "--socket", path, env=env).stdout.count("not up") == count + 1


@needs_aterm
def test_the_free_text_card_allows_other_and_the_seat_prints_beside_it(daemon) -> None:
    path, env = daemon
    script("up", "--socket", path, env=env)
    ask = next(f["ask"] for f in Watcher(path).frames() if f.get("type") == "ask" and f["ask"]["from"] == "fixture Free text")
    assert ask.get("allowOther") or ask.get("allow_other"), ask
    status = subprocess.run(["aterm", "status", "fixture-free"], env=env, capture_output=True, text=True, check=False, timeout=20)
    assert "output while the card is open" in status.stdout


@needs_aterm
def test_a_toggler_raises_its_card_and_then_ends_it_on_its_own(daemon) -> None:
    path, env = daemon
    script("up", "--socket", path, env=env)
    asked: dict[str, str] = {}
    for f in Watcher(path).frames():
        if f.get("type") == "ask" and f["ask"]["from"] == "fixture Flip A":
            asked[f["ask"]["id"]] = "open"
        if f.get("type") == "asked" and f.get("ask_id") in asked and f.get("state") == "cancelled":
            return
    raise AssertionError("the card never ended by itself")


@needs_aterm
def test_the_reply_seat_publishes_last_reply_and_the_others_do_not(daemon) -> None:
    path, env = daemon
    script("up", "--socket", path, env=env)
    watcher = Watcher(path)
    if "last-reply" not in watcher.welcome.get("features", []):
        pytest.skip("this aterm daemon predates last_reply")
    seen: dict[str, str] = {}
    for f in watcher.frames():
        for row in f.get("sessions") or []:
            seen[row["name"]] = row.get("last_reply", "")
        if seen.get("fixture-reply"):
            break
    assert seen["fixture-reply"] == "Fixture reply: the build is green."
    assert not seen.get("fixture-quiet")
    watcher.send({"type": "last_reply", "id": "r", "target": "fixture-reply"})
    reply = next(f for f in watcher.frames() if f.get("type") == "last_reply")
    assert "second paragraph" in reply["reply"]["text"]


@needs_aterm
def test_the_wide_client_holds_the_wide_seat_at_its_columns_until_down(daemon) -> None:
    path, env = daemon
    script("up", "--socket", path, env=env)
    assert "wide client up" in script("status", "--socket", path, env=env).stdout
    # A phone-sized client attaches, and the daemon says the largest client's size.
    phone = Watcher(path)
    phone.send({"type": "attach", "id": "p", "session": fixtures.WIDE_SEAT, "rows": 24, "cols": 40, "scales": True})
    size = next(f for f in phone.frames() if f.get("type") == "size")
    assert size["cols"] == fixtures.WIDE_COLS, size
    assert "wide client stopped" in script("down", "--socket", path, env=env).stdout
    assert "wide client not up" in script("status", "--socket", path, env=env).stdout


def _screen(seat: str, env: dict[str, str], want: str) -> str:
    for _ in range(20):
        shown = subprocess.run(["aterm", "status", seat], env=env, capture_output=True, text=True, check=False, timeout=20).stdout
        if want in shown:
            return shown
        time.sleep(0.5)
    return shown


@needs_aterm
def test_the_alt_screen_seat_fills_its_screen_and_reads_the_wheel(daemon) -> None:
    path, env = daemon
    script("up", "--socket", path, env=env)
    shown = _screen("fixture-alt", env, "wheel:none")
    assert "alt row 20" in shown and "wheel:none" in shown, shown


@needs_aterm
def test_the_mouse_seat_keeps_a_long_history_on_the_normal_screen(daemon) -> None:
    path, env = daemon
    script("up", "--socket", path, env=env)
    shown = _screen("fixture-mouse", env, "history line 200")
    assert "history line 200" in shown, shown
