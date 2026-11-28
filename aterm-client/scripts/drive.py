# /// script
# requires-python = ">=3.12"
# dependencies = ["playwright==1.59.0"]
# ///
"""Drive the real aterm client against the dev mock host, headless, and assert the screen.

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
# A real 1x1 PNG, so the mock host reads it as an image by its bytes.
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
    page.get_by_role("tab", name="Mock host").click()
    # A desktop switches seats by tab, a phone by a row in the seat list.
    # A phone row's name opens with the tab's name, which tells two Beetle-Ox seats apart.
    tab = page.get_by_role("tab", name=seat)
    (tab if tab.count() else page.get_by_role("button", name=re.compile("^" + re.escape(seat)))).first.click()
    page.wait_for_timeout(1800)
    return page


def side_sheet(page, tag: str, expect) -> None:
    """Below 1000px: the strip clears the gesture bar, stays up mid-draft, and a raised sheet reaches the seat bar."""
    field = page.locator("#composer-input")
    field.click()
    field.type("a draft in progress")
    tab = page.locator("#side-tab-messages")
    box = tab.bounding_box()
    height = page.viewport_size["height"]
    expect(f"{tag} side sheet: the strip stays while a draft is in the composer", box is not None and tab.is_visible(), box)
    expect(f"{tag} side sheet: the strip clears the bottom 24px", box is not None and height - (box["y"] + box["height"]) >= 24, box)
    tab.click()
    page.wait_for_timeout(300)
    tops = page.evaluate("() => [document.querySelector('.session .body'), document.querySelector('aside.panel')].map((e) => Math.round(e.getBoundingClientRect().top))")
    expect(f"{tag} side sheet: a raised sheet reaches the seat bar", tops[0] == tops[1], tops)
    page.screenshot(path=str(CLIENT / f"drive-{page.viewport_size['width']}-sheet.png"))
    page.keyboard.press("Escape")
    expect(f"{tag} side sheet: Escape puts it away", page.locator("aside.panel").get_attribute("data-open") == "false", None)
    field.fill("")


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
    expect(f"{tag} feedback: a send names its record", "fb-mock-" in done, done)


def sign_in(browser, url: str, size: tuple[int, int], tag: str, expect) -> None:
    """The signed-out seat: the card, the hop to the browser, a refused code, and a code that works (COI-3678, COI-3679)."""
    page = browser.new_page(viewport={"width": size[0], "height": size[1]})
    # Nothing navigates: window.open is replaced by a stub that records the address.
    page.add_init_script("window.__opened = []; window.open = (u) => { window.__opened.push(String(u)); return null; };")
    page.goto(url)
    page.get_by_role("tab", name="Mock host").click()
    # A desktop switches seats by tab, a phone by a row in the seat list.
    tab = page.get_by_role("tab", name="Heron-Ox")
    (tab if tab.count() else page.get_by_role("button", name=re.compile("Heron-Ox"))).first.click()
    page.wait_for_timeout(1800)
    card = page.locator("[data-card='login']")

    def rows() -> str:
        return page.evaluate("() => [...document.querySelectorAll('.xterm-rows > div')].map((r) => r.textContent).join('\\n')")

    expect(f"{tag} signed-out seat: the native sign-in card is up", card.count() == 1 and card.get_by_role("button", name="Sign in with Claude").is_visible(), card.count())
    page.screenshot(path=str(CLIENT / f"drive-{size[0]}-login-method.png"))
    page.get_by_role("button", name="Show terminal").click()
    expect(f"{tag} signed-out seat: the terminal's own menu is reachable under the card", "Select login method" in rows() and page.get_by_role("button", name="Show sign-in").is_visible(), rows()[:200])
    page.get_by_role("button", name="Show sign-in").click()
    page.get_by_role("button", name="Sign in with Claude").click()
    page.wait_for_timeout(900)
    opened = page.evaluate("() => window.__opened")
    expect(f"{tag} one tap opens the sign-in page in the browser, not the streamed one", len(opened) == 1 and opened[0].startswith("https://claude.com/cai/oauth/authorize?"), opened)
    expect(f"{tag} the sign-in card now asks for the code", page.locator("#login-code").is_visible(), card.inner_text()[:120])
    field = page.locator("#login-code")
    box = field.bounding_box() or {"height": 0}
    expect(f"{tag} the code field is a 44px target", box["height"] >= 44, box)
    expect(f"{tag} no sideways scroll", page.evaluate("() => document.documentElement.scrollWidth <= document.documentElement.clientWidth"), size)
    page.screenshot(path=str(CLIENT / f"drive-{size[0]}-login-code.png"))
    field.fill("not-the-code#nope")
    field.press("Enter")
    page.wait_for_timeout(900)
    refused = page.get_by_role("alert").filter(has_text="That code wasn't accepted")
    expect(f"{tag} a wrong code leaves the seat on its login screen with the error on the card", refused.count() == 1 and "OAuth error" in rows(), rows()[:300])
    page.screenshot(path=str(CLIENT / f"drive-{size[0]}-login-error.png"))
    page.get_by_role("button", name="Try again").click()
    page.wait_for_timeout(700)
    page.get_by_role("button", name="Sign in with Claude").click()
    page.wait_for_timeout(900)
    page.locator("#login-code").fill("  demo-code#demo-state ")
    page.get_by_role("button", name="Sign in", exact=True).click()
    page.wait_for_timeout(1200)
    expect(f"{tag} the made-up code reaches the logged-in prompt", card.count() == 0 and "auto mode on" in rows(), rows()[-200:])
    page.close()


def host_kind(browser, url: str, size: tuple[int, int], tag: str, expect) -> None:
    """The host kind (COI-3778), worst state first: a locked device sees it and cannot set it."""
    page = browser.new_page(viewport={"width": size[0], "height": size[1]})
    page.goto(url + "?terminals=locked")
    page.get_by_role("tab", name=re.compile("^Mock host")).click()
    page.wait_for_timeout(600)
    picker = page.locator("#machine-kind")
    expect(f"{tag} host kind locked: the kind shows", picker.count() == 1 and picker.input_value() == "personal", picker.count())
    expect(f"{tag} host kind locked: it cannot be changed", picker.count() == 1 and picker.is_disabled(), None)
    hint = page.locator("#machine-kind-hint").inner_text() if picker.count() else ""
    expect(f"{tag} host kind locked: the hint says why", "read only" in hint, hint)
    page.screenshot(path=str(CLIENT / f"drive-{size[0]}-kind-locked.png"))
    page.close()

    page = browser.new_page(viewport={"width": size[0], "height": size[1]})
    page.goto(url)
    row = page.get_by_role("tab", name=re.compile("^Mock host"))
    row.click()
    page.wait_for_timeout(600)
    # A phone's host rows are chips with no detail, so only a desktop row names the kind.
    chip = size[0] <= 720
    if not chip:
        expect(f"{tag} host kind: the row names the kind", "Personal //" in row.inner_text(), row.inner_text())
    page.get_by_role("button", name="Settings", exact=True).first.click()
    picker = page.get_by_role("combobox", name="Host kind // Mock host")
    expect(f"{tag} host kind: Settings offers the select", picker.count() == 1 and picker.is_enabled(), picker.count())
    picker.select_option("durable")
    page.wait_for_timeout(600)
    expect(f"{tag} host kind: the host keeps the change", picker.input_value() == "durable", picker.input_value())
    if not chip:
        expect(f"{tag} host kind: the row follows it", "Durable //" in row.inner_text(), row.inner_text())
    width = page.evaluate("() => document.documentElement.scrollWidth")
    expect(f"{tag} host kind: no sideways scroll", width <= size[0], width)
    picker.scroll_into_view_if_needed()
    page.screenshot(path=str(CLIENT / f"drive-{size[0]}-kind.png"))
    page.close()


def dead_seats(browser, url: str, size: tuple[int, int], tag: str, expect) -> None:
    """A seat whose holder died (COI-3799): listed under Dead, reopened from the keyboard."""
    page = browser.new_page(viewport={"width": size[0], "height": size[1]})
    page.goto(url + "?dead=1")
    page.get_by_role("tab", name=re.compile("^Mock host")).click()
    page.wait_for_timeout(600)
    heading = page.get_by_role("heading", name=re.compile("^Dead"))
    expect(f"{tag} dead seat: the Dead group shows", heading.count() >= 1 and heading.first.is_visible(), heading.count())
    row = page.locator("ul.dead li").first
    text = row.inner_text() if row.count() else ""
    expect(f"{tag} dead seat: the row names the seat and when it was found", "Sturdy-Imp" in text and "found dead 3h ago" in text, text)
    width = page.evaluate("() => document.documentElement.scrollWidth")
    expect(f"{tag} dead seat: no sideways scroll", width <= size[0], width)
    page.screenshot(path=str(CLIENT / f"drive-{size[0]}-dead.png"))
    reached = False
    for _ in range(80):
        page.keyboard.press("Tab")
        if page.evaluate("() => document.activeElement?.getAttribute('aria-label')") == "Reopen Sturdy-Imp":
            reached = True
            break
    expect(f"{tag} dead seat: Tab reaches Reopen", reached, None)
    if reached:
        page.keyboard.press("Enter")
        page.wait_for_timeout(600)
        expect(f"{tag} dead seat: Reopen takes it off the list", page.get_by_role("heading", name=re.compile("^Dead")).count() == 0, None)
        page.screenshot(path=str(CLIENT / f"drive-{size[0]}-dead-reopened.png"))
    page.close()


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
                sign_in(browser, f"http://localhost:{port}/", size, f"{size[0]}px", expect)
            for size in WIDTHS:
                host_kind(browser, f"http://localhost:{port}/", size, f"{size[0]}px", expect)
            for size in WIDTHS:
                dead_seats(browser, f"http://localhost:{port}/", size, f"{size[0]}px", expect)
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
                if size[0] <= 1000:
                    side_sheet(page, tag, expect)
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
