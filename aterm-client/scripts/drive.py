# /// script
# requires-python = ">=3.12"
# dependencies = ["playwright==1.59.0"]
# ///
"""Drive the real aterm client against the Demo host, headless, and assert the screen.

Serves its own dev server on a free port, so another seat's server is never tested.
Reference: tooling-aterm-client, "One input at every width".
"""
import base64
import json
import re
import socket
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

from playwright.sync_api import sync_playwright

CLIENT = Path(__file__).resolve().parent.parent
WIDTHS = [(1280, 800), (320, 640)]
# A real 1x1 PNG, so the Demo host reads it as an image by its bytes.
PIXEL = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
PROBE = """() => {
  const seen = (e) => { const r = e.getBoundingClientRect(); const s = getComputedStyle(e); return r.width > 4 && r.height > 4 && s.visibility !== 'hidden' && s.opacity !== '0'; };
  const screen = document.querySelector('.xterm');
  const clip = screen ? parseFloat((screen.style.clipPath.match(/ ([0-9.]+)px\\)/) || [0, 0])[1]) : 0;
  const host = document.querySelector('.term-host');
  const limit = Math.min(screen ? screen.getBoundingClientRect().bottom - clip : 0, host ? host.getBoundingClientRect().bottom : 0);
  const rows = [...document.querySelectorAll('.xterm-rows > div')];
  const shows = (re) => rows.some((r) => re.test(r.textContent) && r.getBoundingClientRect().bottom <= limit + 1);
  return {
    textBoxes: [...document.querySelectorAll('textarea')].filter(seen).map((e) => e.id || e.className),
    harnessBox: shows(/auto mode on|seat ─/),
    menu: shows(/Run this command/),
    card: !!document.querySelector('.overlay'),
  };
}"""


def free_port() -> int:
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return probe.getsockname()[1]


def serve(port: int) -> subprocess.Popen:
    server = subprocess.Popen(["pnpm", "run", "dev", "--port", str(port), "--strictPort"], cwd=CLIENT, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(100):
        try:
            urllib.request.urlopen(f"http://localhost:{port}/", timeout=1)
            return server
        except OSError:
            time.sleep(0.2)
    server.terminate()
    sys.exit("the dev server never answered")


def open_seat(browser, url: str, size: tuple[int, int], seat: str):
    page = browser.new_page(viewport={"width": size[0], "height": size[1]})
    page.goto(url)
    page.get_by_role("tab", name="Demo host").click()
    page.get_by_role("tab", name=seat).first.click()
    page.wait_for_timeout(1800)
    return page


def open_entry(page) -> None:
    """A phone has the icon in its bar, one tap. A desktop has the sidebar's entry."""
    icon = page.locator("button.icon[aria-label='Send feedback']")
    (icon if icon.count() else page.locator("nav button.entry")).click()


def feedback(page, tag: str, expect) -> None:
    """The feedback sheet, worst states first: a refused screenshot, an empty note, then a send."""
    icon = page.locator("button.icon[aria-label='Send feedback']")
    if page.viewport_size["width"] <= 720:
        target = icon.bounding_box()
        expect(f"{tag} feedback: the phone bar has the icon, with no menu open", icon.is_visible() and not page.locator("#sidebar-menu").count(), None)
        expect(f"{tag} feedback: the icon is a 44px touch target", target is not None and target["width"] >= 44 and target["height"] >= 44, target)
    open_entry(page)
    expect(f"{tag} feedback: one tap opens the sheet", page.get_by_role("dialog", name="Send feedback").is_visible(), None)
    # The heading names the dialog and reads Sent after a send, so find it by being open.
    sheet = page.locator("dialog[open]")
    line = sheet.locator(".sent-with").inner_text()
    expect(f"{tag} feedback: the sheet says what goes along", "Imp-Dragonfly" in line and "build" in line, line)
    send = sheet.get_by_role("button", name="Send", exact=True)
    expect(f"{tag} feedback: Send waits for something to send", send.is_disabled(), None)
    note = sheet.get_by_label("What should change on this screen?")
    note.fill("The tab strip sits on the last line")
    expect(f"{tag} feedback: a note enables Send", send.is_enabled(), None)
    sheet.locator("input[type=file]").set_input_files({"name": "fake.png", "mimeType": "image/png", "buffer": b"not an image at all"})
    send.click()
    refusal = sheet.get_by_role("alert").inner_text()
    expect(f"{tag} feedback: a refused screenshot says why and keeps the draft", "Not sent" in refusal and "stay here" in refusal, refusal)
    expect(f"{tag} feedback: the note survives the refusal", note.input_value() != "", None)
    sheet.get_by_role("button", name=re.compile("Remove screenshot 1")).click()
    sheet.locator("input[type=file]").set_input_files({"name": "screen.png", "mimeType": "image/png", "buffer": PIXEL})
    sheet.get_by_role("list", name="Screenshots to send").wait_for()
    box = sheet.bounding_box()
    width = page.viewport_size["width"]
    expect(f"{tag} feedback: the sheet fits the screen", box is not None and box["x"] >= 0 and box["x"] + box["width"] <= width + 1 and box["y"] >= 0, box)
    expect(f"{tag} feedback: no sideways scroll", page.evaluate("document.documentElement.scrollWidth <= innerWidth + 1"), None)
    page.screenshot(path=str(CLIENT / f"drive-{width}-feedback.png"))
    sheet.get_by_role("button", name="Close").click()
    open_entry(page)
    kept = note.input_value()
    expect(f"{tag} feedback: closing keeps the draft", kept == "The tab strip sits on the last line", kept)
    send.click()
    sheet.get_by_role("heading", name="Sent").wait_for()
    done = sheet.get_by_role("status").inner_text()
    expect(f"{tag} feedback: a send names its record", "fb-demo-" in done, done)


def main() -> int:
    port = free_port()
    server = serve(port)
    failures: list[str] = []

    def expect(label: str, ok: bool, detail: object) -> None:
        print(("pass " if ok else "FAIL ") + label + ("" if ok else f"  {detail}"))
        if not ok:
            failures.append(label)

    try:
        with sync_playwright() as p:
            browser = p.chromium.launch()
            for size in WIDTHS:
                tag = f"{size[0]}px"
                page = open_seat(browser, f"http://localhost:{port}/", size, "Imp-Dragonfly")
                seen = page.evaluate(PROBE)
                expect(f"{tag} resting seat: the composer is the only text box", seen["textBoxes"] == ["composer-input"], seen["textBoxes"])
                expect(f"{tag} resting seat: the harness's own input box is out of view", not seen["harnessBox"], seen)
                field = page.locator("#composer-input")
                field.click()
                field.type("one")
                field.press("Shift+Enter")
                field.type("two")
                expect(f"{tag} Shift+Enter adds a line", field.input_value() == "one\ntwo", field.input_value())
                field.press("Enter")
                page.wait_for_timeout(400)
                expect(f"{tag} Enter sends and empties the field", field.input_value() == "", field.input_value())
                page.screenshot(path=str(CLIENT / f"drive-{size[0]}-resting.png"))
                feedback(page, tag, expect)
                page.close()
                page = open_seat(browser, f"http://localhost:{port}/", size, "Beetle-Ox, Platform Engineer on claude")
                seen = page.evaluate(PROBE)
                expect(f"{tag} permission menu: the menu stays on screen", seen["menu"], seen)
                expect(f"{tag} permission menu: the native choice card is offered", seen["card"], seen)
                page.screenshot(path=str(CLIENT / f"drive-{size[0]}-menu.png"))
                page.close()
            browser.close()
    finally:
        server.terminate()
    print(json.dumps({"failed": failures}))
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
