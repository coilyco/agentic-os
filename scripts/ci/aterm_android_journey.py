#!/usr/bin/env python3
"""Drive the debug aterm app on the CI emulator through the image journeys (COI-3485).

A: pick a PNG and Send. B: fire a SEND with a MediaStore URI, cold and warm. Both end with
the daemon's file hashing equal to the PNG (aterm_android_seat.py). D: observe Open in
browser. E: the relay stub refuses a send, and the page says so. C: stop the daemon. The
page is driven over scripts/aterm_devtools.py, the picker through uiautomator.
Design: tooling-aterm-client references/android.md. Standard library only.
"""
from __future__ import annotations

import argparse
import base64
import importlib.util
import json
import os
import re
import struct
import subprocess
import sys
import time
import urllib.request
import xml.etree.ElementTree as ET
import zlib
from pathlib import Path

APP = "dev.coilyco.treehouse"
HERE = Path(__file__).resolve().parent
PNG_NAME = "ci-seed.png"
# The composer's chip for the first picked image, and its Send button (frontend-eng).
CHIP = 'button[aria-label="Remove image 1"]'
SEND = "form.composer button.primary"
# Fragments of imageRefusal() in aterm-client/src/lib/images.ts, which frontend-eng owns.
REFUSAL_WORDS = {"too_large": r"over 3 MB", "not_an_image": r"PNG, JPEG, GIF or WebP"}


def _shared_devtools():
    """The seat verb's DevTools client, loaded by path because `python3 -I` drops this directory from sys.path."""
    spec = importlib.util.spec_from_file_location("aterm_devtools", HERE.parent / "aterm_devtools.py")
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


devtools = _shared_devtools()
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
    """A small PNG with a signature the daemon accepts, made here so no binary is tracked.

    `red` makes each journey's file hash differ from the others.
    """
    def chunk(kind: bytes, body: bytes) -> bytes:
        return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body) & 0xFFFFFFFF)
    rows = b"".join(b"\x00" + bytes([red, 30, 30]) * 64 for _ in range(64))
    path.write_bytes(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 64, 64, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b""))


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

    def attach_page(self) -> None:
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            sockets = re.findall(r"@(webview_devtools_remote_\d+)", adb("shell", "cat", "/proc/net/unix", check=False))
            if sockets:
                adb("forward", "tcp:9222", f"localabstract:{sockets[0]}")
                try:
                    targets = json.load(urllib.request.urlopen("http://127.0.0.1:9222/json", timeout=5))
                except OSError:
                    targets = []
                url = devtools.choose_page(targets, 9222, app_only=True)
                if url:
                    self.page = Page(url)
                    return
            time.sleep(1)
        raise Fail("no debuggable page at tauri.localhost: the WebView is not debuggable or the app did not start")

    def add_host_and_open_seat(self) -> None:
        page = self.page
        assert page
        page.wait_text("Add a host")
        page.js("(()=>{const i=document.querySelector('#add-host');i.value='ws://127.0.0.1:7419';i.dispatchEvent(new Event('input',{bubbles:true}))})()")
        page.tap(by_text("Add host", "button"))
        page.tap("()=>document.querySelector('[id^=tab-host-]')")
        # The seat button is named by role and state, its own name label is empty.
        page.tap(by_text("frontend-eng", "nav button"))

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

    def pick_image(self) -> None:
        """Pick the seeded PNG: a Photo Picker thumbnail if it lists one, else Browse to the file."""
        def thumb(node: ET.Element) -> bool:
            named = PNG_NAME in (node.get("text", "") + node.get("content-desc", ""))
            return named or node.get("resource-id", "").endswith(("icon_thumb", "icon_thumbnail", "item_root"))
        if self.tap_node(thumb, 12):
            # The app's input allows several files, so the picker waits for Add (1).
            self.tap_node(lambda n: n.get("resource-id", "").endswith("button_add"), 8)
            return
        print("the Photo Picker lists no image, so browsing to the file")
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
            ("scan-file", ("shell", "am", "broadcast", "-a", "android.intent.action.MEDIA_SCANNER_SCAN_FILE", "-d", f"file:///sdcard/Pictures/{name}")),
            ("scan-volume", ("shell", "content", "call", "--uri", "content://media/", "--method", "scan_volume", "--arg", "external_primary")),
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
        adb("shell", "am", "start", "-a", "android.intent.action.SEND", "-t", "image/png", "--eu", "android.intent.extra.STREAM", uri, "--grant-read-uri-permission", "-n", f"{APP}/.MainActivity")
        if cold:
            self.page = None
            self.attach_page()
        page = self.page
        assert page
        page.wait_text("Send to a seat", 30)
        page.js("(()=>{const t=document.querySelector('[role=dialog] textarea,[role=dialog] input[type=text]');if(t){t.value='look at this';t.dispatchEvent(new Event('input',{bubbles:true}))}})()")
        if "Pick a seat" in page.text():
            page.tap("()=>document.querySelector('[role=dialog] input[type=radio]')")
        page.tap(by_text("Send to ", "button"))
        self.shot(f"sheet-sent-{name}")
        self.daemon_got_the_bytes(name)

    def journey_d_open_in_browser(self) -> None:
        """Observe only: what a tap on an artifact card's Open in browser does on Android (COI-3520)."""
        page = self.page
        assert page
        Path(self.args.seat_log + ".open").write_text("https://claude.ai/artifact/ci-observe")
        card = 'section[aria-label="A seat opened a page"] button'
        deadline = time.monotonic() + 30
        while not page.js(f"!!document.querySelector({json.dumps(card)})"):
            if time.monotonic() > deadline:
                raise Fail(f"no artifact card appeared. page text: {page.text()[:300]!r}")
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
        report = f"focus before: {before}\nfocus after: {self.focus()}\nresumed: {resumed}\npage url after: {href}\n"
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
        for finder in ("()=>document.querySelector('[id^=tab-host-]')", by_text("frontend-eng", "nav button")):
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

    def journey_c_daemon_down(self) -> None:
        """Evidence only: the failure copy is the client's to specify (COI-3485)."""
        page = self.page
        assert page
        self.open_picker()
        self.pick_image()
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

    def journey_f_dictation(self) -> None:
        """The mic taps, a spoken phrase plays into the virtual mic, and text lands in the composer (COI-3460)."""
        page = self.page
        assert page
        mic = '[aria-label="Dictate"]'
        stop = '[aria-label="Stop dictating"]'
        field = "document.querySelector('#composer-input')"
        wav = os.environ.get("ATERM_SPEECH_WAV")
        if not wav or not Path(wav).exists():
            raise Fail("no speech file, so the virtual mic was never set up (ATERM_SPEECH_WAV)")
        page.js(f"(()=>{{const f={field};f.value='';f.dispatchEvent(new Event('input',{{bubbles:true}}))}})()")
        page.wait_selector(mic, 20)
        page.tap(f"()=>document.querySelector({json.dumps(mic)})")
        page.wait_selector(stop, 20)
        time.sleep(2)
        subprocess.run(["paplay", "--device=virt_mic", wav], check=False, timeout=60)
        deadline = time.monotonic() + 45
        heard = ""
        while time.monotonic() < deadline and not re.search(r"hello", heard, re.IGNORECASE):
            time.sleep(1)
            heard = str(page.js(f"{field}.value"))
        (self.out / "journey-f-composer.txt").write_text(heard)
        logs = [line for line in adb("logcat", "-d", "-t", "6000", check=False).splitlines() if re.search(r"speech|recogni|dictation|SpeechRecognizer", line, re.IGNORECASE)]
        (self.out / "journey-f-logcat.txt").write_text("\n".join(logs[-300:]))
        if not re.search(r"hello", heard, re.IGNORECASE):
            raise Fail(f"no words reached the composer from the spoken phrase. composer: {heard!r}. page notes: {page.text()[:300]!r}")
        if page.js(f"!!document.querySelector({json.dumps(stop)})"):
            page.tap(f"()=>document.querySelector({json.dumps(stop)})")
            page.wait_selector(mic, 20)
        print(f"journey F: the composer holds {str(page.js(f'{field}.value'))!r}")

    def run(self) -> None:
        self.step("install-and-launch", self.install_and_launch)
        self.step("attach-devtools", self.attach_page)
        self.step("add-host-and-open-seat", self.add_host_and_open_seat)
        self.step("tap-add-image-picker-opens", self.open_picker)
        self.step("pick-the-seeded-png", self.pick_image)
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
        if os.environ.get("ATERM_JOURNEY_DICTATION"):
            try:
                self.step("journey-f-dictation", self.journey_f_dictation)
            except SystemExit:
                if os.environ["ATERM_JOURNEY_DICTATION"] != "optional":
                    raise
                print("::warning::journey F failed and is optional (COI-3460), see its FAILED screenshot")
                self.recover()
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
