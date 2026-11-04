# /// script
# requires-python = ">=3.12"
# dependencies = ["playwright==1.59.0"]
# ///
"""Drive the real aterm client against the Demo host, headless, and assert the screen.

Serves its own dev server on a free port, so another seat's server is never tested.
Reference: tooling-aterm-client, "One input at every width".
"""
import json
import socket
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

from playwright.sync_api import sync_playwright

CLIENT = Path(__file__).resolve().parent.parent
WIDTHS = [(1280, 800), (320, 640)]
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
