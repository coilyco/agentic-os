#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Drive one Android emulator running the real Treehouse app, for a Frontend seat.

One verb, `just aterm-android-emulator <command>`. Reference and exit codes:
tooling-aterm-client references/android-emulator.md. Every address is a setting.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
import time
import urllib.request
from dataclasses import dataclass, field
from pathlib import Path

import aterm_android_media
import aterm_devtools

EXIT_USAGE, EXIT_FAIL, EXIT_LOCK, EXIT_PRESSURE, EXIT_NO_DEVICE = 2, 1, 6, 7, 8
LOCK_DIR = "/data/local/tmp/treehouse-emulator.lock"
APP_ID = "dev.coilyco.treehouse"


class Refused(Exception):
    """A request the verb declines, carrying the exit code to return."""

    def __init__(self, message: str, code: int = EXIT_FAIL):
        super().__init__(message)
        self.code = code


def _env_int(env: dict[str, str], key: str, default: int) -> int:
    return int(env.get(key) or default)


@dataclass(frozen=True)
class Config:
    """Every setting the verb reads, from TREEHOUSE_EMULATOR_* variables."""

    adb: str = "adb"
    adb_host: str = "127.0.0.1"
    adb_port: int = 5037
    ssh: str = ""
    ssh_adb_port: int = 5037
    devtools_port: int = 9222
    ssh_devtools_port: int = 9222
    serial: str = "emulator-5554"
    avd: str = "Pixel_9"
    unit: str = "treehouse-emulator.service"
    stop_reason_path: str = "/var/lib/android-emu/guard-stop"
    holder: str = ""
    lock_ttl: int = 900
    boot_timeout: int = 240
    max_swap_pct: int = 75
    max_load_per_core: float = 2.0
    env: dict[str, str] = field(default_factory=dict, compare=False)

    @classmethod
    def from_env(cls, env: dict[str, str]) -> Config:
        """With an ssh target the local ports are the tunnel's ends, so they differ."""
        key = "TREEHOUSE_EMULATOR_"
        ssh = env.get(key + "SSH", "")
        return cls(
            adb=env.get(key + "ADB", "adb"),
            adb_host=env.get(key + "ADB_HOST", "127.0.0.1"),
            adb_port=_env_int(env, key + "ADB_PORT", 15037 if ssh else 5037),
            ssh=ssh,
            ssh_adb_port=_env_int(env, key + "SSH_ADB_PORT", 5037),
            devtools_port=_env_int(env, key + "DEVTOOLS_PORT", 19222 if ssh else 9222),
            ssh_devtools_port=_env_int(env, key + "SSH_DEVTOOLS_PORT", 9222),
            serial=env.get(key + "SERIAL", "emulator-5554"),
            avd=env.get(key + "AVD", "Pixel_9"),
            unit=env.get(key + "UNIT", "treehouse-emulator.service"),
            stop_reason_path=env.get(key + "STOP_REASON", "/var/lib/android-emu/guard-stop"),
            holder=env.get(key + "HOLDER") or env.get("AOS_NATIVE_SESSION", ""),
            lock_ttl=_env_int(env, key + "LOCK_TTL", 900),
            boot_timeout=_env_int(env, key + "BOOT_TIMEOUT", 240),
            max_swap_pct=_env_int(env, key + "MAX_SWAP_PCT", 75),
            max_load_per_core=float(env.get(key + "MAX_LOAD_PER_CORE") or 2.0),
            env=env,
        )


def run(cmd: list[str], *, check: bool = True, env: dict[str, str] | None = None, binary: bool = False):
    """Run a command and return the finished process, raising Refused on failure."""
    done = subprocess.run(cmd, capture_output=True, text=not binary, check=False, env=env)
    if check and done.returncode != 0:
        err = done.stderr if not binary else done.stderr.decode(errors="replace")
        raise Refused(f"{' '.join(cmd[:4])} failed ({done.returncode}): {err.strip()}")
    return done


class Emulator:
    """One emulator reached through one adb server, local or through an ssh tunnel."""

    def __init__(self, cfg: Config, out=print):
        self.cfg = cfg
        self.out = out

    def adb_cmd(self, *args: str, serial: bool = True) -> list[str]:
        base = [self.cfg.adb, "-H", self.cfg.adb_host, "-P", str(self.cfg.adb_port)]
        return base + (["-s", self.cfg.serial] if serial else []) + list(args)

    def adb(self, *args: str, check: bool = True, serial: bool = True, binary: bool = False):
        return run(self.adb_cmd(*args, serial=serial), check=check, binary=binary)

    def shell(self, command: str, check: bool = True) -> str:
        return self.adb("shell", command, check=check).stdout.strip()

    # Reaching the adb server, local or at the far end of an ssh tunnel

    def tunnel_end_closed(self) -> bool:
        """A socket test, because adb -H on a loopback address starts its own server."""
        import socket

        with socket.socket() as sock:
            sock.settimeout(2)
            return sock.connect_ex((self.cfg.adb_host, self.cfg.adb_port)) != 0

    def ensure_tunnel(self) -> None:
        """Open the ssh tunnel to the host's adb server and DevTools port when it is closed."""
        c = self.cfg
        if not c.ssh or not self.tunnel_end_closed():
            return
        cmd = ["ssh", "-f", "-N", "-o", "BatchMode=yes", "-o", "ExitOnForwardFailure=yes", "-o", "ServerAliveInterval=15"]
        cmd += ["-L", f"{c.adb_host}:{c.adb_port}:127.0.0.1:{c.ssh_adb_port}"]
        cmd += ["-L", f"127.0.0.1:{c.devtools_port}:127.0.0.1:{c.ssh_devtools_port}", c.ssh]
        # ssh -f leaves a child holding a pipe, so its output goes to a file.
        with tempfile.TemporaryFile() as err:
            done = subprocess.run(cmd, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=err, check=False)
            err.seek(0)
            if done.returncode != 0:
                raise Refused(f"ssh tunnel to {c.ssh} failed ({done.returncode}): {err.read().decode(errors='replace').strip()}")
        for _ in range(20):
            if not self.tunnel_end_closed():
                return
            time.sleep(0.5)
        raise Refused(f"the ssh tunnel to {c.ssh} started but {c.adb_host}:{c.adb_port} never opened", EXIT_NO_DEVICE)

    def stop_reason(self) -> str | None:
        """Why the host stopped the emulator, when it published a reason."""
        if not self.cfg.ssh:
            return None
        done = run(["ssh", "-o", "BatchMode=yes", self.cfg.ssh, f"cat {self.cfg.stop_reason_path}"], check=False)
        return done.stdout.strip() or None if done.returncode == 0 else None

    def booted(self) -> bool:
        done = self.adb("shell", "getprop sys.boot_completed", check=False)
        return done.returncode == 0 and done.stdout.strip() == "1"

    def need_device(self) -> None:
        self.ensure_tunnel()
        if not self.booted():
            why = f", the host stopped it: {self.stop_reason()}" if self.stop_reason() else ""
            raise Refused(f"no booted emulator {self.cfg.serial}{why}, run up", EXIT_NO_DEVICE)

    # The lock, held on the emulator so it binds every seat wherever the emulator runs

    def holder(self) -> str:
        if not self.cfg.holder:
            raise Refused("set TREEHOUSE_EMULATOR_HOLDER, or run in a native session so AOS_NATIVE_SESSION is set", EXIT_USAGE)
        return self.cfg.holder

    def lock_owner(self) -> tuple[str, int] | None:
        """The holder and the seconds since the lock was last touched, or None when free."""
        text = self.shell(f"cat {LOCK_DIR}/owner 2>/dev/null; echo; stat -c %Y {LOCK_DIR} 2>/dev/null; date +%s", check=False)
        lines = [line for line in text.splitlines()]
        if len(lines) < 3 or not lines[0].strip():
            return None
        return lines[0].strip(), int(lines[-1]) - int(lines[-2])

    def acquire(self, wait: int = 0) -> str:
        who = self.holder()
        deadline = time.monotonic() + wait
        while True:
            if self.adb("shell", f"mkdir {LOCK_DIR}", check=False).returncode == 0:
                self.shell(f"echo {who} > {LOCK_DIR}/owner")
                return f"acquired by {who}"
            current = self.lock_owner()
            if current is None:
                self.shell(f"rm -rf {LOCK_DIR}")
                continue
            owner, idle = current
            if owner == who:
                self.shell(f"touch {LOCK_DIR}")
                return f"already held by {who}"
            if idle > self.cfg.lock_ttl:
                self.out(f"taking the lock over from {owner}, idle {idle}s over the {self.cfg.lock_ttl}s limit")
                self.shell(f"rm -rf {LOCK_DIR}")
                continue
            if time.monotonic() >= deadline:
                raise Refused(f"the emulator is held by {owner}, last active {idle}s ago, free after {self.cfg.lock_ttl}s idle", EXIT_LOCK)
            time.sleep(5)

    def need_lock(self) -> None:
        """Every driving command holds the lock, and each one counts as activity."""
        who = self.holder()
        current = self.lock_owner()
        if current is None or current[0] != who:
            held = f"held by {current[0]}" if current else "not held"
            raise Refused(f"the emulator lock is {held}, run acquire first", EXIT_LOCK)
        self.shell(f"touch {LOCK_DIR}")

    def release(self, force: bool = False) -> str:
        current = self.lock_owner()
        if current is None:
            return "lock was free"
        if current[0] != self.holder() and not force:
            raise Refused(f"the lock is held by {current[0]}, not {self.holder()}", EXIT_LOCK)
        self.shell(f"rm -rf {LOCK_DIR}")
        return "released"

    # Pressure, read from the host that runs the emulator

    def host_text(self, command: str) -> str:
        if self.cfg.ssh:
            return run(["ssh", "-o", "BatchMode=yes", self.cfg.ssh, command]).stdout
        return run(["sh", "-c", command]).stdout

    def pressure(self) -> dict[str, float]:
        """Swap used as a percent, and load per core, on macOS or Linux."""
        system = self.host_text("uname -s").strip()
        if system == "Darwin":
            raw = self.host_text("sysctl -n vm.swapusage; sysctl -n vm.loadavg; sysctl -n hw.ncpu").splitlines()
            total, used = (float(x) for x in re.findall(r"(?:total|used) = ([0-9.]+)M", raw[0]))
            load, cores = float(raw[1].strip("{} ").split()[0]), int(raw[2])
        else:
            raw = self.host_text("cat /proc/loadavg; nproc; grep -E '^Swap(Total|Free):' /proc/meminfo")
            lines = raw.splitlines()
            load, cores = float(lines[0].split()[0]), int(lines[1])
            kib = {k: int(v) for k, v in re.findall(r"(Swap\w+):\s+(\d+) kB", raw)}
            total, used = kib["SwapTotal"], kib["SwapTotal"] - kib["SwapFree"]
        return {"swap_pct": round(100 * used / total, 1) if total else 0.0, "load_per_core": round(load / cores, 2)}

    def check_pressure(self) -> dict[str, float]:
        got = self.pressure()
        if got["swap_pct"] > self.cfg.max_swap_pct or got["load_per_core"] > self.cfg.max_load_per_core:
            limits = f"{self.cfg.max_swap_pct}% swap, {self.cfg.max_load_per_core} load per core"
            raise Refused(f"not booting while the host is under pressure: {got}, limits {limits}", EXIT_PRESSURE)
        return got

    # Bringing it up and down

    def systemctl(self, action: str) -> None:
        """The exact argv the host's sudoers rule matches, so no other form works."""
        run(["ssh", "-o", "BatchMode=yes", self.cfg.ssh, "sudo", "-n", "/bin/systemctl", action, self.cfg.unit])

    def up(self) -> str:
        self.ensure_tunnel()
        if self.booted():
            return f"already up: {self.cfg.serial}"
        got = self.check_pressure()
        c = self.cfg
        if c.ssh:
            self.systemctl("start")
        else:
            home = Path(os.path.expanduser(f"~{os.environ.get('USER', '')}"))
            env = os.environ | {"ANDROID_AVD_HOME": str(home / ".android" / "avd")}
            run(["android", "emulator", "start", "--headless", c.avd], env=env)
        deadline = time.monotonic() + c.boot_timeout
        while time.monotonic() < deadline:
            self.ensure_tunnel()
            if self.booted():
                return f"up: {c.serial}, host pressure at start {got}"
            time.sleep(3)
        raise Refused(f"{c.serial} did not finish booting in {c.boot_timeout}s", EXIT_NO_DEVICE)

    def down(self) -> str:
        if self.cfg.ssh:
            self.systemctl("stop")
        else:
            run(["android", "emulator", "stop", self.cfg.avd], check=False)
            run(["pkill", "-f", "qemu-system-aarch64-headless"], check=False)
        return "stopped"

    # Driving the app

    def install(self, apk: str, reinstall: bool = False) -> str:
        if not Path(apk).is_file():
            raise Refused(f"no such APK: {apk}", EXIT_USAGE)
        if reinstall:
            self.adb("uninstall", APP_ID, check=False)
        done = self.adb("install", "-r", "-t", apk, check=False)
        if done.returncode != 0:
            text = (done.stdout + done.stderr).strip()
            if "INSTALL_FAILED_UPDATE_INCOMPATIBLE" in text:
                raise Refused("the installed app has another signing key, each GitHub debug build has its own: "
                              "run install --reinstall, which clears the app's data")
            raise Refused(f"install failed: {text}")
        return f"installed {apk}"

    def pids(self) -> list[str]:
        """Every process the app has, since a stale one can sort before the live one."""
        return self.shell(f"pidof {APP_ID}", check=False).split()

    def keyboard_shown(self) -> bool:
        return "mInputShown=true" in self.shell("dumpsys input_method | grep -m1 mInputShown", check=False)

    def devtools(self) -> str:
        """Forward the app's WebView DevTools socket and print where the page list is."""
        c = self.cfg
        deadline = time.monotonic() + float(c.env.get("TREEHOUSE_EMULATOR_DEVTOOLS_WAIT") or 15)
        while True:
            pids, table = self.pids(), self.shell("cat /proc/net/unix")
            want = aterm_devtools.app_socket(pids, table)
            if want:
                break
            if not pids:
                raise Refused(f"{APP_ID} is not running, run open", EXIT_NO_DEVICE)
            # The WebView is made as the activity starts, so a fresh launch can be early.
            if time.monotonic() >= deadline:
                listed = ", ".join(aterm_devtools.listed_sockets(table)) or "none"
                raise Refused(
                    f"{APP_ID} (pids {', '.join(pids)}) exposes no webview_devtools_remote_<pid>. Sockets listed: {listed}. "
                    "The release build has no WebView DevTools, so install the debug APK",
                    EXIT_NO_DEVICE,
                )
            time.sleep(1)
        remote = c.ssh_devtools_port if c.ssh else c.devtools_port
        self.adb("forward", f"tcp:{remote}", f"localabstract:{want}")
        return f"http://127.0.0.1:{c.devtools_port}"

    def page(self) -> str:
        """The debugger URL of the app's page, on the local end of the DevTools forward."""
        base = self.devtools()
        with urllib.request.urlopen(f"{base}/json/list", timeout=10) as resp:
            url = aterm_devtools.choose_page(json.load(resp), self.cfg.devtools_port)
        if not url:
            raise Refused("the app's WebView has no page yet", EXIT_NO_DEVICE)
        return url

    def evaluate(self, expression: str):
        try:
            with aterm_devtools.Page(self.page()) as page:
                return page.js(expression)
        except aterm_devtools.DevToolsError as err:
            raise Refused(str(err)) from err

    def webview_bounds(self) -> tuple[int, int]:
        """The WebView's top-left on screen, from the UI tree, in device pixels."""
        self.shell("uiautomator dump /sdcard/treehouse-ui.xml >/dev/null")
        xml = self.shell("cat /sdcard/treehouse-ui.xml")
        found = re.search(r'class="android\.webkit\.WebView"[^>]*bounds="\[(\d+),(\d+)\]', xml)
        if not found:
            raise Refused("no android.webkit.WebView in the UI tree", EXIT_NO_DEVICE)
        return int(found.group(1)), int(found.group(2))

    def media_uri(self, name: str, wait: float) -> str | None:
        """The MediaStore row for a picture by its file name, once it is listed."""
        query = f"content query --uri content://media/external/images/media --projection _id:_display_name --where \"_display_name='{name}'\""
        deadline = time.monotonic() + wait
        while True:
            found = re.search(r"_id=(\d+)", self.shell(query, check=False))
            if found:
                return f"content://media/external/images/media/{found.group(1)}"
            if time.monotonic() >= deadline:
                return None
            time.sleep(1)

    def photo(self, name: str) -> str:
        """Put a picture in the library so the Photo Picker lists it (COI-3573)."""
        if not re.fullmatch(r"[A-Za-z0-9_.-]+\.png", name):
            raise Refused("a photo name is letters, digits, dot, dash and underscore, ending in .png", EXIT_USAGE)
        wait = float(self.cfg.env.get("TREEHOUSE_EMULATOR_MEDIA_WAIT") or 12)
        with tempfile.TemporaryDirectory() as folder:
            local = Path(folder) / name
            local.write_bytes(aterm_android_media.png(red=40, size=512))
            self.adb("push", str(local), f"/sdcard/Pictures/{name}")
        tried = []
        for label, command in aterm_android_media.scan_attempts(name):
            tried.append(f"{label}: {self.adb(*command, check=False).stdout.strip()[:120]!r}")
            if uri := self.media_uri(name, wait):
                return f"{name} is in the media library as {uri}, found by {label}"
        raise Refused(f"MediaStore never listed {name}. " + " | ".join(tried))

    def tap_selector(self, selector: str) -> str:
        js = (
            "(() => { const e = document.querySelector(" + json.dumps(selector) + "); if (!e) return null;"
            " const r = e.getBoundingClientRect(); return {x: r.left + r.width / 2, y: r.top + r.height / 2, w: r.width, h: r.height, dpr: window.devicePixelRatio}; })()"
        )
        point = self.evaluate(js)
        if not point:
            raise Refused(f"no element matches {selector}", EXIT_FAIL)
        if point["w"] <= 0 or point["h"] <= 0:
            raise Refused(f"the first match for {selector} has no size on screen, a tap would hit the corner", EXIT_FAIL)
        left, top = self.webview_bounds()
        x, y = round(left + point["x"] * point["dpr"]), round(top + point["y"] * point["dpr"])
        self.shell(f"input tap {x} {y}")
        return f"tapped {selector} at {x},{y}"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="aterm-android-emulator", description=(__doc__ or "").split("\n\n")[0])
    sub = parser.add_subparsers(dest="command", required=True)
    for name, args in {
        "status": [], "up": [], "down": [], "release": [], "open": [], "devtools": [], "dom": ["selector"],
        "install": ["apk"], "tap": ["x", "y"], "tap-selector": ["selector"], "type": ["text"],
        "key": ["name"], "keyboard": ["action"], "shot": ["out"], "eval": ["js"], "reverse": ["port"],
        "acquire": [], "tunnel": [], "stop-app": [], "clear-app": [], "photo": [],
    }.items():
        p = sub.add_parser(name)
        for arg in args:
            p.add_argument(arg)
        if name == "acquire":
            p.add_argument("--wait", type=int, default=0, help="seconds to wait for the lock")
        if name == "release":
            p.add_argument("--force", action="store_true")
        if name == "photo":
            p.add_argument("--name", default="treehouse-fixture.png", help="the file name in /sdcard/Pictures")
        if name == "install":
            p.add_argument("--reinstall", action="store_true", help="uninstall first, which clears the app's data")
    return parser


def dispatch(emu: Emulator, args: argparse.Namespace) -> str:
    cmd = args.command
    if cmd == "tunnel":
        emu.ensure_tunnel()
        return f"tunnel to {emu.cfg.ssh} ready" if emu.cfg.ssh else "adb is local, no tunnel needed"
    if cmd == "up":
        return emu.up()
    if cmd == "down":
        return emu.down()
    if cmd == "status":
        emu.ensure_tunnel()
        up = emu.booted()
        lock = emu.lock_owner() if up else None
        return json.dumps({"adb": f"{emu.cfg.adb_host}:{emu.cfg.adb_port}", "ssh": emu.cfg.ssh or None, "serial": emu.cfg.serial,
                           "booted": up, "lock": {"holder": lock[0], "idle_s": lock[1]} if lock else None,
                           "host_stop_reason": None if up else emu.stop_reason(), "pressure": emu.pressure()})
    emu.need_device()
    if cmd == "acquire":
        return emu.acquire(args.wait)
    if cmd == "release":
        return emu.release(args.force)
    emu.need_lock()
    if cmd == "install":
        return emu.install(args.apk, args.reinstall)
    if cmd == "open":
        emu.shell(f"monkey -p {APP_ID} -c android.intent.category.LAUNCHER 1")
        return f"opened {APP_ID}"
    if cmd == "stop-app":
        emu.shell(f"am force-stop {APP_ID}")
        return "stopped the app"
    if cmd == "clear-app":
        emu.shell(f"pm clear {APP_ID}")
        return "cleared the app's data"
    if cmd == "tap":
        emu.shell(f"input tap {int(args.x)} {int(args.y)}")
        return f"tapped {args.x},{args.y}"
    if cmd == "type":
        emu.shell("input text " + "'" + args.text.replace(" ", "%s").replace("'", "") + "'")
        return "typed"
    if cmd == "key":
        emu.shell(f"input keyevent {args.name}")
        return f"key {args.name}"
    if cmd == "keyboard":
        if args.action not in ("state", "close"):
            raise Refused("keyboard takes state or close, open it by tapping a field", EXIT_USAGE)
        if args.action == "close" and emu.keyboard_shown():
            emu.shell("input keyevent 4")
        return "keyboard shown" if emu.keyboard_shown() else "keyboard hidden"
    if cmd == "shot":
        png = emu.adb("exec-out", "screencap", "-p", binary=True).stdout
        Path(args.out).write_bytes(png)
        return f"{args.out} {len(png)} bytes"
    if cmd == "photo":
        return emu.photo(args.name)
    if cmd == "reverse":
        emu.adb("reverse", f"tcp:{args.port}", f"tcp:{args.port}")
        return f"emulator localhost:{args.port} now reaches port {args.port} on the host running the adb server, as loopback"
    if cmd == "devtools":
        return emu.devtools()
    if cmd == "eval":
        return json.dumps(emu.evaluate(args.js))
    if cmd == "dom":
        return str(emu.evaluate(f"(document.querySelector({json.dumps(args.selector)}) || {{}}).outerHTML || null"))
    if cmd == "tap-selector":
        return emu.tap_selector(args.selector)
    raise Refused(f"unknown command {cmd}", EXIT_USAGE)


def main(argv: list[str] | None = None, env: dict[str, str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        print(dispatch(Emulator(Config.from_env(dict(os.environ if env is None else env))), args))
    except Refused as err:
        print(f"aterm-android-emulator: {err}", file=sys.stderr)
        return err.code
    return 0


if __name__ == "__main__":
    sys.exit(main())
