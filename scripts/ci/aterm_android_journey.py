#!/usr/bin/env python3
"""Drive the debug aterm app on the CI emulator through the image journeys (COI-3485).

A: the picker opens, a PNG goes in by the file input, Send. B: fire a SEND with a
MediaStore URI, cold and warm. Both end with the daemon's file hashing equal to the PNG
(aterm_android_seat.py). D: observe Open in browser. E: the relay stub refuses a send.
Picker: a real pick, warning only (COI-3580). C: stop the daemon. The page is driven
over scripts/aterm_devtools.py, the picker through uiautomator. Standard library only.
Design: tooling-aterm-client references/android.md.
"""
from __future__ import annotations

import argparse
import base64
import importlib.util
import json
import os
import re
import subprocess
import sys
import time
import urllib.request
import xml.etree.ElementTree as ET
from pathlib import Path

APP = "dev.coilyco.treehouse"
HERE = Path(__file__).resolve().parent
PNG_NAME = "ci-seed.png"
# The composer's chip for the first picked image, and its Send button (frontend-eng).
CHIP = 'button[aria-label="Remove image 1"]'
SEND = "form.composer button.primary"
# Fragments of imageRefusal() in aterm-client/src/lib/images.ts, which frontend-eng owns.
REFUSAL_WORDS = {"too_large": r"over 3 MB", "not_an_image": r"PNG, JPEG, GIF or WebP"}


def _shared(name: str):
    """A module the seat verb shares, loaded by path because `python3 -I` drops this directory from sys.path."""
    spec = importlib.util.spec_from_file_location(name, HERE.parent / f"{name}.py")
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


devtools = _shared("aterm_devtools")
media = _shared("aterm_android_media")
Page = devtools.Page
# One error type for the job and the shared client, so a step catches either the same way.
Fail = devtools.DevToolsError


def adb(*args: str, check: bool = True, timeout: int = 60) -> str:
    done = subprocess.run(["adb", *args], capture_output=True, text=True, timeout=timeout, check=False)
    if check and done.returncode != 0:
        raise Fail(f"adb {' '.join(args)} failed: {done.stderr.strip() or done.stdout.strip()}")
    return done.stdout


def adb_all(*args: str, stdin: Path | None = None, timeout: int = 60) -> str:
    """Stdout and stderr of an adb call, for diagnosis, never raising on a failed command."""
    with (stdin.open("rb") if stdin else open(os.devnull, "rb")) as feed:
        done = subprocess.run(["adb", *args], stdin=feed, capture_output=True, text=True, timeout=timeout, check=False)
    return (done.stdout + done.stderr).strip()


def seed_png(path: Path, red: int = 220) -> None:
    path.write_bytes(media.png(red))


def by_text(text: str, selector: str = "button,[role=tab],[role=button],a,li,[aria-label]") -> str:
    """Finder for the first element whose text or aria-label holds text."""
    return f"()=>[...document.querySelectorAll({json.dumps(selector)})].find(e=>(e.textContent+' '+(e.getAttribute('aria-label')||'')).includes({json.dumps(text)}))"


class Journey:
    def __init__(self, args: argparse.Namespace) -> None:
        self.args = args
        self.out = Path(args.out)
        self.out.mkdir(parents=True, exist_ok=True)
        self.shots = 0
        self.page: Page | None = None

    def shot(self, name: str) -> None:
        self.shots += 1
        data = subprocess.run(["adb", "exec-out", "screencap", "-p"], capture_output=True, check=False, timeout=30).stdout
        (self.out / f"{self.shots:02d}-{name}.png").write_bytes(data)

    def step(self, name: str, fn) -> None:
        print(f"== {name}", flush=True)
        try:
            fn()
        except (Fail, OSError, subprocess.TimeoutExpired) as err:
            self.shot(f"FAILED-{name}")
            self.evidence()
            print(f"FAILED {name}: {err}", file=sys.stderr)
            raise SystemExit(1) from err
        self.shot(name)

    def evidence(self) -> None:
        (self.out / "window.txt").write_text(adb("shell", "dumpsys", "window", check=False))
        (self.out / "logcat.txt").write_text(adb("logcat", "-d", "-t", "4000", check=False))
        if self.page:
            try:
                (self.out / "page.txt").write_text(self.page.text())
            except (Fail, OSError):
                pass
        (self.out / "ui.xml").write_text(self.ui_xml())

    def ui_xml(self) -> str:
        adb("shell", "uiautomator", "dump", "/sdcard/ui.xml", check=False)
        return adb("exec-out", "cat", "/sdcard/ui.xml", check=False)

    def focus(self) -> str:
        """The focused window and the resumed activity, since an error dialog can take focus."""
        window = [line for line in adb("shell", "dumpsys", "window").splitlines() if "mCurrentFocus" in line]
        resumed = [line for line in adb("shell", "dumpsys", "activity", "activities").splitlines() if "ResumedActivity" in line]
        return "\n".join(window + resumed[:2])

    def install_and_launch(self) -> None:
        adb("reverse", "tcp:7419", "tcp:7419")
        adb("install", "-r", "-g", self.args.apk, timeout=180)
        self.seed(PNG_NAME, 220)
        adb("shell", "am", "start", "-n", f"{APP}/.MainActivity")

    def devtools_state(self) -> str:
        """The app's pids beside the DevTools sockets Android lists, which says which one is missing (COI-3600)."""
        pids = adb("shell", "pidof", APP, check=False).strip() or "none"
        sockets = devtools.listed_sockets(adb("shell", "cat", "/proc/net/unix", check=False))
        return f"app pids: {pids}. DevTools sockets: {', '.join(sockets) or 'none'}"

    def attach_page(self) -> None:
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            # By the app's pid, since a Google-APIs image has other WebView hosts.
            socket_name = devtools.app_socket(adb("shell", "pidof", APP, check=False).split(), adb("shell", "cat", "/proc/net/unix", check=False))
            if socket_name:
                adb("forward", "tcp:9222", f"localabstract:{socket_name}")
                try:
                    targets = json.load(urllib.request.urlopen("http://127.0.0.1:9222/json", timeout=5))
                except OSError:
                    targets = []
                url = devtools.choose_page(targets, 9222, app_only=True)
                if url:
                    self.page = Page(url)
                    return
            time.sleep(1)
        raise Fail("no debuggable page at tauri.localhost: the WebView is not debuggable or the app did not start. " + self.devtools_state())

    def add_host_and_open_seat(self) -> None:
        page = self.page
        assert page
        page.wait_text("Add a host")
        page.js("(()=>{const i=document.querySelector('#add-host');i.value='ws://127.0.0.1:7419';i.dispatchEvent(new Event('input',{bubbles:true}))})()")
        page.tap(by_text("Add host", "button"))
        page.tap("()=>document.querySelector('[id^=tab-host-]')")
        self.open_seat()

    def composer_visible(self) -> bool:
        """The message box is on screen. It can be mounted under the Seats overview and not be."""
        assert self.page
        return bool(self.page.js("(()=>{const e=document.querySelector('#composer-input');return !!e&&e.getClientRects().length>0})()"))

    def open_seat(self) -> None:
        """Tap the seat's overview row, or its nav button, until the composer is on screen."""
        page = self.page
        assert page
        # The row moves groups as the seat settles and can swallow a tap, so tap again.
        for _ in range(4):
            page.tap(by_text("frontend-eng", ".overview button.open, nav button"))
            deadline = time.monotonic() + 6
            while time.monotonic() < deadline:
                if self.composer_visible():
                    return
                time.sleep(0.5)
            print("open_seat: the seat did not open, tapping its row again")
        raise Fail("the seat never opened from its overview row")

    def open_picker(self) -> None:
        page = self.page
        assert page
        page.tap("()=>document.querySelector('[aria-label=\"Add image\"]')")
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            # Android 13 and later answers GET_CONTENT for images with its Photo Picker.
            if any(name in self.focus() for name in ("documentsui", "photopicker", "providers.media")):
                return
            time.sleep(0.5)
        raise Fail(f"no system picker in front. focus: {self.focus() or 'none'}")

    def dismiss_picker(self) -> None:
        """Back out of the system picker, so the page gets its file input's cancel and the app is in front."""
        adb("shell", "input", "keyevent", "4")
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if APP in self.focus() and not any(name in self.focus() for name in ("documentsui", "photopicker", "providers.media")):
                return
            time.sleep(0.5)
        raise Fail(f"the picker did not close. focus: {self.focus() or 'none'}")

    def tap_node(self, wanted, wait: float) -> bool:
        """Tap the centre of the first uiautomator node `wanted` accepts, within wait seconds."""
        deadline = time.monotonic() + wait
        while time.monotonic() < deadline:
            # uiautomator's dump from the emulator this job started, not untrusted XML.
            tree = ET.fromstring(self.ui_xml() or "<hierarchy/>")
            for node in tree.iter("node"):
                if wanted(node):
                    x0, y0, x1, y1 = map(int, re.findall(r"\d+", node.get("bounds", "0,0,0,0")))
                    adb("shell", "input", "tap", str((x0 + x1) // 2), str((y0 + y1) // 2))
                    return True
            time.sleep(1)
        return False

    def pick_image(self, mode: str = "browse") -> None:
        """Pick the seeded PNG: `browse` goes through the file browser, `thumb` takes the Photo Picker's own."""
        def thumb(node: ET.Element) -> bool:
            named = PNG_NAME in (node.get("text", "") + node.get("content-desc", ""))
            return named or node.get("resource-id", "").endswith(("icon_thumb", "icon_thumbnail", "item_root"))
        if mode == "thumb":
            if not self.tap_node(thumb, 25):
                raise Fail("the Photo Picker listed no image to take")
            # The app's input allows several files, so the picker waits for Add (1).
            self.tap_node(lambda n: n.get("resource-id", "").endswith("button_add"), 8)
            return
        if not self.tap_node(lambda n: n.get("content-desc") == "More options", 10):
            raise Fail("no More options in the picker, ui.xml has what it did show")
        if not self.tap_node(lambda n: n.get("text", "").startswith("Browse"), 10):
            raise Fail("no Browse item in the picker menu, ui.xml has what it did show")
        # A grid item names the file in its content description, not its text.
        if not self.tap_node(lambda n: n.get("content-desc", "").startswith(f"{PNG_NAME},") or n.get("text") == PNG_NAME, 30):
            raise Fail(f"the file browser did not show {PNG_NAME}, ui.xml has what it did show")

    def chip_then_send(self) -> None:
        page = self.page
        assert page
        page.wait_selector(CHIP)
        page.tap(f"()=>document.querySelector({json.dumps(SEND)})")

    def daemon_got_the_bytes(self, name: str = PNG_NAME) -> None:
        png = self.out / name
        done = subprocess.run([sys.executable, "-I", str(HERE / "aterm_android_seat.py"), "expect-image", self.args.socket, self.args.seat, str(png), self.args.seat_log, "--timeout", "30"], capture_output=True, text=True, check=False)
        print(done.stdout + done.stderr, end="")
        if done.returncode != 0:
            raise Fail(f"the daemon did not receive the image: {done.stderr.strip()}")

    def seed(self, name: str, red: int) -> None:
        """Put a distinct PNG in MediaStore, trying three ways until it is listed, so the picker shows it."""
        png = self.out / name
        seed_png(png, red)
        adb("push", str(png), f"/sdcard/Pictures/{name}")
        tried: list[str] = []
        attempts = (
            *media.scan_attempts(name),
            ("insert", ("shell", "content", "insert", "--uri", "content://media/external/images/media", "--bind", f"_display_name:s:{name}", "--bind", "mime_type:s:image/png", "--bind", "relative_path:s:Pictures/ci")),
        )
        for label, command in attempts:
            tried.append(f"{label}: {adb_all(*command)[:160]!r}")
            uri = self.find_media_uri(name, 12)
            if uri:
                if label == "insert":
                    tried.append("write: " + adb_all("shell", "content", "write", "--uri", uri, stdin=png)[:160])
                print(f"indexed {name} by {label}")
                return
        listing = adb_all("shell", "ls", "-l", "/sdcard/Pictures")[:300]
        raise Fail(f"MediaStore never listed {name}. " + " | ".join(tried) + f" | ls: {listing!r}")

    def find_media_uri(self, name: str, wait: float) -> str | None:
        deadline = time.monotonic() + wait
        while time.monotonic() < deadline:
            # adb joins its arguments for the device shell, so quote the SQL here.
            row = adb_all("shell", f"content query --uri content://media/external/images/media --projection _id:_display_name --where \"_display_name='{name}'\"")
            found = re.search(r"_id=(\d+)", row)
            if found:
                return f"content://media/external/images/media/{found.group(1)}"
            time.sleep(1)
        return None

    def media_uri(self, name: str) -> str:
        uri = self.find_media_uri(name, 30)
        if not uri:
            raise Fail(f"MediaStore never listed {name}")
        return uri

    def share_target_present(self) -> bool:
        """Journey B runs when the build registers a SEND target. ATERM_JOURNEY_B=required makes absence a failure."""
        if APP in adb("shell", "cmd", "package", "query-activities", "-a", "android.intent.action.SEND", "-t", "image/png", check=False):
            return True
        if os.environ.get("ATERM_JOURNEY_B") == "required":
            raise SystemExit("journey B: the build has no SEND target and ATERM_JOURNEY_B=required")
        print("::warning::journey B skipped: this build registers no SEND target yet (COI-3485)")
        return False

    def share_target_is_registered(self) -> None:
        for action, mime in (("android.intent.action.SEND", "image/png"), ("android.intent.action.SEND_MULTIPLE", "image/*")):
            listed = adb("shell", "cmd", "package", "query-activities", "-a", action, "-t", mime)
            if APP not in listed:
                raise Fail(f"{APP} is not a target for {action} {mime}. query-activities said: {listed.strip()[:200]!r}")

    def share(self, name: str, cold: bool) -> None:
        """Fire a SEND for the named image, then answer the sheet and Send."""
        if cold:
            adb("shell", "am", "force-stop", APP)
        uri = self.media_uri(name)
        # `am` builds no ClipData, which carries a share's grant, so the URI is data too.
        adb("shell", "am", "start", "-a", "android.intent.action.SEND", "-t", "image/png", "-d", uri, "--eu", "android.intent.extra.STREAM", uri, "--grant-read-uri-permission", "-n", f"{APP}/.MainActivity")
        self.record_grants(name, uri)
        for attempt in (1, 2):
            try:
                if cold or attempt == 2:
                    self.page = None
                    self.attach_page()
                assert self.page
                self.page.wait_text("Send to a seat", 30)
                break
            except Fail as err:
                # The page reloads as the share lands, which can close the first socket.
                if attempt == 2 or "closed the socket" not in str(err):
                    raise
        page = self.page
        assert page
        if "Nothing is left to send" in page.text():
            raise Fail(f"the sheet took nothing from {uri}. page text: {page.text()[:500]!r}")
        page.js("(()=>{const t=document.querySelector('[role=dialog] textarea,[role=dialog] input[type=text]');if(t){t.value='look at this';t.dispatchEvent(new Event('input',{bubbles:true}))}})()")
        if "Pick a seat" in page.text():
            page.tap("()=>document.querySelector('[role=dialog] input[type=radio]')")
        page.tap(by_text("Send to ", "button"))
        self.shot(f"sheet-sent-{name}")
        self.daemon_got_the_bytes(name)

    def record_grants(self, name: str, uri: str) -> None:
        """Keep what Android says the app may read, so a share that took nothing can be told from one that was refused."""
        dump = adb_all("shell", "dumpsys", "activity", "permissions", timeout=60)
        seen = [line for line in dump.splitlines() if "treehouse" in line or uri.rsplit("/", 1)[-1] in line]
        (self.out / f"share-{name}.grants.txt").write_text("\n".join(seen[:60]) or "no grant mentions the app or the row\n")

    def open_facts(self) -> str:
        """What the daemon offered and did for the open, so a missing card says which half failed (COI-3559)."""
        seat = HERE / "aterm_android_seat.py"
        done = subprocess.run([sys.executable, "-I", str(seat), "features", self.args.socket], capture_output=True, text=True, check=False)
        log = Path(self.args.out) / "daemon.log"
        asked = [line for line in log.read_text(errors="replace").splitlines() if "asked to open an address" in line] if log.exists() else []
        return f"welcome features: {done.stdout.strip() or done.stderr.strip()}\ndaemon open log: {asked[-1] if asked else 'no open_url was asked of the daemon'}\n"

    def journey_d_open_in_browser(self) -> None:
        """Observe only: what a tap on an artifact card's Open in browser does on Android (COI-3520)."""
        page = self.page
        assert page
        Path(self.args.seat_log + ".open").write_text("https://claude.ai/artifact/ci-observe")
        card = 'section[aria-label="A seat opened a page"] button'
        deadline = time.monotonic() + 30
        while not page.js(f"!!document.querySelector({json.dumps(card)})"):
            if time.monotonic() > deadline:
                raise Fail(f"no artifact card appeared. {self.open_facts()}page text: {page.text()[:300]!r}")
            time.sleep(0.5)
        self.shot("card-before-tap")
        before = self.focus()
        page.tap(f"()=>document.querySelector({json.dumps(card)})")
        time.sleep(4)
        resumed = [line.strip() for line in adb("shell", "dumpsys", "activity", "activities", check=False).splitlines() if "ResumedActivity" in line][:2]
        try:
            href = page.js("location.href")
        except (Fail, OSError) as err:
            href = f"page not reachable: {err}"
        report = f"{self.open_facts()}focus before: {before}\nfocus after: {self.focus()}\nresumed: {resumed}\npage url after: {href}\n"
        (self.out / "journey-d.txt").write_text(report)
        print("journey D (observation only):\n" + report)
        adb("shell", "am", "start", "-n", f"{APP}/.MainActivity")

    def recover(self) -> None:
        """Bring the app back after a failed step, best effort, so the journeys after it can run."""
        adb("shell", "am", "force-stop", APP, check=False)
        adb("shell", "am", "start", "-n", f"{APP}/.MainActivity", check=False)
        self.page = None
        self.attach_page()
        page = self.page
        assert page
        # The page may already be on a seat view with the nav collapsed, so try briefly.
        for finder in ("()=>document.querySelector('[id^=tab-host-]')", by_text("frontend-eng", ".overview button.open, nav button")):
            try:
                page.tap(finder, wait=5)
            except Fail:
                print("recover: nothing to tap, carrying on with the page as it is")

    def journey_e_refusals(self) -> None:
        """The daemon's too_large and not_an_image refusals reach the person and nothing reaches the seat (COI-3526).

        The real daemon cannot say either to this app, so the relay answers them while the control file names one.
        """
        page = self.page
        assert page
        control = Path(self.args.refuse_file)
        typed_before = Path(self.args.seat_log).read_text(errors="replace")
        self.add_image_without_picker()
        page.wait_selector(CHIP)
        try:
            for reason, words in REFUSAL_WORDS.items():
                control.write_text(reason)
                page.tap(f"()=>document.querySelector({json.dumps(SEND)})")
                self.wait_words(rf"Not sent\..*{words}", 20, reason)
                if not page.js(f"!!document.querySelector({json.dumps(CHIP)})"):
                    raise Fail(f"{reason}: the picture was dropped, the person must keep it")
                self.shot(f"refused-{reason}")
        finally:
            control.unlink(missing_ok=True)
        if Path(self.args.seat_log).read_text(errors="replace") != typed_before:
            raise Fail("a refused upload still typed something into the seat")
        page.tap(f"()=>document.querySelector({json.dumps(CHIP)})")
        print("journey E: both refusals said, picture kept, seat untouched")

    def add_image_without_picker(self) -> None:
        """Hand the composer's file input the seeded PNG. The Photo Picker URI a refusal test would pick is unreadable (run 42896175645)."""
        page = self.page
        assert page
        data = base64.b64encode((self.out / PNG_NAME).read_bytes()).decode()
        page.js(
            "(()=>{const b=Uint8Array.from(atob(" + json.dumps(data) + "),c=>c.charCodeAt(0));"
            "const t=new DataTransfer();t.items.add(new File([b],'ci-refusal.png',{type:'image/png'}));"
            "const i=document.querySelector('form.composer input[type=file]');i.files=t.files;"
            "i.dispatchEvent(new Event('change',{bubbles:true}))})()"
        )

    def wait_words(self, pattern: str, wait: float, label: str) -> None:
        deadline = time.monotonic() + wait
        page = self.page
        assert page
        while time.monotonic() < deadline:
            if re.search(pattern, page.text(), re.IGNORECASE | re.DOTALL):
                return
            time.sleep(0.5)
        raise Fail(f"{label}: the page never matched {pattern!r}. page text: {page.text()[:400]!r}")

    def record_picker(self, label: str) -> None:
        """What Android holds for the app around a pick: grants, activity lifecycle, and related log lines."""
        grants = [line for line in adb_all("shell", "dumpsys", "activity", "permissions", timeout=60).splitlines() if "treehouse" in line or "picker" in line]
        events = [line for line in adb_all("logcat", "-b", "events", "-d", "-v", "time", timeout=60).splitlines() if "treehouse" in line and "wm_" in line]
        log = [line for line in adb_all("logcat", "-d", "-t", "1500", "-v", "time", timeout=60).splitlines() if re.search(r"chromium|FileChooser|Permission Denial|UriGrants|photopicker|NotReadable", line, re.I)]
        body = ["== uri grants", *grants[:60], "== activity events (wm_*)", *events[-40:], "== log", *log[-60:]]
        (self.out / f"picker-{label}.txt").write_text("\n".join(body) + "\n")

    def journey_picker(self, mode: str) -> None:
        """A real pick through the system picker, then Send: does the page read what it hands back (COI-3580)?"""
        page = self.page
        assert page
        before = Path(self.args.seat_log).read_text(errors="replace").count("\n")
        self.open_picker()
        self.pick_image(mode)
        page.wait_selector(CHIP)
        self.record_picker(f"{mode}-after-pick")
        page.tap(f"()=>document.querySelector({json.dumps(SEND)})")
        deadline = time.monotonic() + 20
        said = ""
        while time.monotonic() < deadline:
            found = re.search(r"Not sent\.[^\n]*", page.text())
            said = found.group(0) if found else ""
            if said or Path(self.args.seat_log).read_text(errors="replace").count("\n") > before:
                break
            time.sleep(0.5)
        self.record_picker(f"{mode}-after-send")
        if said:
            if page.js(f"!!document.querySelector({json.dumps(CHIP)})"):
                page.tap(f"()=>document.querySelector({json.dumps(CHIP)})")
            raise Fail(f"the page could not send the {mode} pick: {said!r}")
        if Path(self.args.seat_log).read_text(errors="replace").count("\n") <= before:
            raise Fail("nothing reached the seat and the page said nothing")
        print(f"journey {mode}: the picked image reached the seat")

    def journey_c_daemon_down(self) -> None:
        """Evidence only: the failure copy is the client's to specify (COI-3485)."""
        page = self.page
        assert page
        self.add_image_without_picker()
        page.wait_selector(CHIP)
        subprocess.run(["pkill", "-f", f"aterm daemon --socket {self.args.socket}"], check=False)
        time.sleep(1)
        page.tap(f"()=>document.querySelector({json.dumps(SEND)})")
        time.sleep(3)
        (self.out / "journey-c-page.txt").write_text(page.text())
        if not page.js(f"!!document.querySelector({json.dumps(CHIP)})"):
            raise Fail("the image chip is gone after a failed send, the draft was not kept")
        words = os.environ.get("ATERM_JOURNEY_C_WORDS")
        if words and not re.search(words, page.text(), re.IGNORECASE):
            raise Fail(f"the page did not say {words!r}")
        print("journey C: chip kept; failure words " + ("asserted" if words else "captured in journey-c-page.txt, not asserted"))

    def speak(self) -> None:
        """Feed the spoken phrase to the emulator's virtual mic over gRPC (aterm_audio_stream.py made it)."""
        stream, grpcurl = os.environ.get("ATERM_SPEECH_JSONL"), os.environ.get("ATERM_GRPCURL")
        if not stream or not grpcurl or not Path(stream).exists():
            raise Fail("no phrase to speak: the job did not run its `audio` step (ATERM_SPEECH_JSONL)")
        port = os.environ.get("ATERM_GRPC_PORT", "8554")
        with open(stream, "rb") as feed:
            done = subprocess.run([grpcurl, "-plaintext", "-d", "@", f"127.0.0.1:{port}", "android.emulation.control.EmulatorController/injectAudio"], stdin=feed, capture_output=True, text=True, timeout=90, check=False)
        if done.returncode != 0:
            raise Fail(f"injectAudio failed: {(done.stderr or done.stdout).strip()[:300]}")

    def dictate(self, label: str, pattern: str, wait: float, speak: bool) -> str:
        """Tap the mic, optionally play the spoken phrase into the virtual mic, and read the composer."""
        page = self.page
        assert page
        mic = '[aria-label="Dictate"]'
        stop = '[aria-label="Stop dictating"]'
        field = "document.querySelector('#composer-input')"
        page.js(f"(()=>{{const f={field};f.value='';f.dispatchEvent(new Event('input',{{bubbles:true}}))}})()")
        page.wait_selector(mic, 20)
        page.tap(f"()=>document.querySelector({json.dumps(mic)})")
        page.wait_selector(stop, 20)
        if speak:
            time.sleep(2)
            self.speak()
        deadline = time.monotonic() + wait
        heard = ""
        while time.monotonic() < deadline and not re.search(pattern, heard, re.IGNORECASE):
            time.sleep(0.5)
            heard = str(page.js(f"{field}.value"))
        (self.out / f"{label}-composer.txt").write_text(heard)
        logs = [line for line in adb("logcat", "-d", "-t", "8000", check=False).splitlines() if re.search(r"speech|recogni|dictation|soda", line, re.IGNORECASE) and "Tauri/Plugin" not in line]
        (self.out / f"{label}-logcat.txt").write_text("\n".join(logs[-300:]))
        if not re.search(pattern, heard, re.IGNORECASE):
            raise Fail(f"{label}: no words reached the composer. composer: {heard!r}. page notes: {page.text()[-300:]!r}")
        return heard

    def end_dictation(self) -> None:
        page = self.page
        assert page
        stop = '[aria-label="Stop dictating"]'
        if page.js(f"!!document.querySelector({json.dumps(stop)})"):
            page.tap(f"()=>document.querySelector({json.dumps(stop)})")
        page.wait_selector('[aria-label="Dictate"]', 20)

    def journey_f_spoken_phrase(self) -> None:
        """The real route: a spoken phrase plays into the virtual mic and the phone's own recognizer hears it (COI-3460)."""
        heard = self.dictate("journey-f", r"hello", 45, speak=True)
        self.end_dictation()
        print(f"journey F: spoken phrase heard as {heard!r}")

    def journey_g_fake_recognizer(self) -> None:
        """The debug build's fake recognition service named as the default: the plugin and page run end to end without audio."""
        adb("shell", "settings", "put", "secure", "voice_recognition_service", f"{APP}/{APP}.dictation.FakeRecognitionService")
        heard = self.dictate("journey-g", r"run the gate and report back", 30, speak=False)
        self.end_dictation()
        page = self.page
        assert page
        # The field stays focused, so the keyboard covers the composer. Hide it.
        if "mInputShown=true" in adb("shell", "dumpsys input_method | grep -m1 mInputShown", check=False):
            adb("shell", "input", "keyevent", "4")
            time.sleep(1)
        self.shot("composer-after-dictation")
        settled = str(page.js("document.querySelector('#composer-input').value"))
        if settled != heard and not settled.startswith("run the gate"):
            raise Fail(f"journey G: the composer changed after the session ended: {settled!r}")
        print(f"journey G: composer holds {settled!r}")

    def run(self) -> None:
        self.step("install-and-launch", self.install_and_launch)
        self.step("attach-devtools", self.attach_page)
        self.step("add-host-and-open-seat", self.add_host_and_open_seat)
        self.step("tap-add-image-picker-opens", self.open_picker)
        self.step("dismiss-the-picker", self.dismiss_picker)
        self.step("add-the-seeded-png", self.add_image_without_picker)
        self.step("chip-then-send", self.chip_then_send)
        self.step("daemon-received-the-bytes", self.daemon_got_the_bytes)
        print("journey A: passed")
        if self.share_target_present():
            try:
                self.step("share-target-registered", self.share_target_is_registered)
                self.step("seed-share-images", lambda: (self.seed("ci-share-cold.png", 90), self.seed("ci-share-warm.png", 150)))
                self.step("share-cold-start", lambda: self.share("ci-share-cold.png", cold=True))
                self.step("share-warm-app-open", lambda: self.share("ci-share-warm.png", cold=False))
                print("journey B: passed")
            except SystemExit:
                if os.environ.get("ATERM_JOURNEY_B") != "optional":
                    raise
                print("::warning::journey B failed and is optional (COI-3485), see its FAILED screenshot")
                self.recover()
        try:
            self.step("journey-d-open-in-browser", self.journey_d_open_in_browser)
        except SystemExit:
            # An observation: what it saw is the result, so a failure is a warning.
            print("::warning::journey D saw no artifact card (COI-3520), see its FAILED screenshot")
            self.recover()
        self.step("journey-e-refusals", self.journey_e_refusals)
        for mode in ("thumb", "browse"):
            try:
                self.step(f"journey-picker-{mode}", lambda: self.journey_picker(mode))
            except SystemExit:
                # COI-3580's evidence is in picker-<mode>-*.txt, so this only warns.
                print(f"::warning::journey picker {mode} failed, see picker-{mode}-*.txt (COI-3580)")
                self.recover()
        if os.environ.get("ATERM_JOURNEY_DICTATION"):
            try:
                self.step("journey-f-spoken-phrase", self.journey_f_spoken_phrase)
            except SystemExit:
                # Observation: Google's recognizer on a CI image may lack a pack or a key.
                print("::warning::journey F heard nothing and is optional (COI-3460), see its FAILED screenshot")
                self.recover()
            self.step("journey-g-fake-recognizer", self.journey_g_fake_recognizer)
        self.step("journey-c-daemon-down", self.journey_c_daemon_down)
        self.evidence()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("apk", "out", "socket", "seat", "seat-log", "refuse-file"):
        parser.add_argument(f"--{name}", required=True)
    Journey(parser.parse_args()).run()
    return 0


if __name__ == "__main__":
    sys.exit(main())
