# /// script
# requires-python = ">=3.12"
# dependencies = ["playwright==1.59.0"]
# ///
"""Drive the connection check against a mock daemon that fails at each hop.

Every scenario breaks one hop on a real socket, so the browser produces the
real failure. The panel's verdict is compared with the net::ERR_* code the
browser itself reports for the failing request, which a page cannot read.
Reference: tooling-aterm-client, references/reach.md.
"""
import json
import os
import re
import socket
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

from playwright.sync_api import sync_playwright

CLIENT = Path(__file__).resolve().parent.parent
SHOTS = CLIENT / "drive-hops"
HERE = Path(__file__).resolve().parent
NUMBER = {"resolve": 1, "route": 2, "tls": 3, "http": 4, "socket": 5, "admit": 6}
# A refused, reset or failed-TLS request is one error to a page, so no narrower claim.
ANY = "Broke at hop 1, 2 or 3. A page cannot tell which."

# name, trusts the certificate, the hop a person would say broke, the verdict owed.
SCENARIOS = [
    ("ok", True, None, "Every hop passed just now."),
    ("unresolved", False, 1, ANY),
    ("refused", False, 2, ANY),
    ("reset", False, 2, ANY),
    ("silent", False, 2, "Most likely broke at hop 2, Tailnet route. A page cannot rule out hop 1 or 3."),
    ("tls-selfsigned", False, 3, ANY),
    ("tls-plain", False, 3, ANY),
    ("http-500", True, 4, "Broke at hop 4, HTTP."),
    ("upgrade-refused", True, 5, "Broke at hop 5, Websocket upgrade."),
    ("opaque-origin", True, 5, "Broke at hop 5, Websocket upgrade."),
    ("refused-origin", True, 5, "Broke at hop 5, Websocket upgrade."),
    ("welcome-error", True, 6, "Broke at hop 6, Daemon admits this device."),
    ("cut-after-open", True, 6, "Broke at hop 6, Daemon admits this device."),
    ("no-welcome", True, 6, "Broke at hop 6, Daemon admits this device."),
    ("refused-ownership", True, 6, "Broke at hop 6, Daemon admits this device."),
    ("refused-name", True, 6, "Broke at hop 6, Daemon admits this device."),
]
# What the browser's own error code says about which hop failed. A page never reads these.
# ERR_ABORTED is the page's own timeout and ERR_FAILED a blocked read: no hop.
NET_HOP = [(r"NAME_NOT_RESOLVED", 1), (r"CONNECTION_(REFUSED|RESET|TIMED_OUT)|SOCKET_NOT_CONNECTED", 2), (r"CERT_|SSL_", 3)]
NOT_A_HOP = r"ERR_ABORTED|ERR_FAILED"


def free_port() -> int:
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return probe.getsockname()[1]


def serve_client(port: int) -> subprocess.Popen:
    server = subprocess.Popen(["pnpm", "run", "dev", "--port", str(port), "--strictPort"], cwd=CLIENT, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(100):
        try:
            urllib.request.urlopen(f"http://localhost:{port}/", timeout=1)
            return server
        except OSError:
            time.sleep(0.2)
    server.terminate()
    sys.exit("the dev server never answered")


def start_daemon() -> tuple[subprocess.Popen, dict[str, int]]:
    daemon = subprocess.Popen([sys.executable, str(HERE / "hop_daemon.py")], stdout=subprocess.PIPE, text=True)
    assert daemon.stdout is not None
    return daemon, json.loads(daemon.stdout.readline())


def hop_numbers(page, *statuses: str) -> set[int]:
    return {NUMBER[li.get_attribute("data-hop")] for status in statuses for li in page.locator(f'li[data-hop][data-status="{status}"]').all()}


def shoot(page, size: tuple[int, int], path: Path) -> None:
    """The whole panel at this width. A short window scrolls the panel, which a capture stitches wrongly, so it is taken tall."""
    page.set_viewport_size({"width": size[0], "height": 1500})
    page.locator(".check").screenshot(path=str(path))
    page.set_viewport_size({"width": size[0], "height": size[1]})
    page.screenshot(path=str(path.with_name(path.stem + "-page.png")))


def seeded(browser, ports, name: str, size: tuple[int, int], clipboard: str = "real"):
    """A page on a mock host, at a size. `clipboard` is "real" or "broken" (a page that may not write it)."""
    context = browser.new_context(viewport={"width": size[0], "height": size[1]}, ignore_https_errors=True, has_touch=size[0] < 500)
    if browser.browser_type.name == "chromium":
        context.grant_permissions(["clipboard-read", "clipboard-write"])
    address = f"wss://127.0.0.1:{ports[name]}/"
    context.add_init_script(f"localStorage.setItem('aterm.hosts.v1', JSON.stringify([{{label: 'mock', address: {json.dumps(address)}}}]))")
    if clipboard == "broken":
        context.add_init_script("Object.defineProperty(navigator, 'clipboard', {value: {writeText: () => Promise.reject(new Error('insecure'))}})")
    return context


def surface(browser, ports, url: str, expect) -> None:
    """What a person sees and does: the worst states first, at a phone width and a desktop width."""
    wait = "() => { const v = document.querySelector('p.verdict'); return v && v.textContent.trim() && !v.textContent.startsWith('Checking'); }"
    for size in [(1280, 800), (320, 640)]:
        tag = f"{size[0]}px"
        # The empty state: a host that answers, before anyone asks.
        context = seeded(browser, ports, "ok", size)
        page = context.new_page()
        page.goto(url)
        page.get_by_role("tab", name="mock").click()
        page.get_by_role("button", name="Check connection").wait_for()
        expect(f"{tag} empty state: no hop rows and an empty status until asked", page.locator("li[data-hop]").count() == 0 and page.locator("p.verdict").inner_text().strip() == "")
        shoot(page, size, SHOTS / f"drive-hops-state-empty-{size[0]}.png")
        context.close()

        # Waiting: a host that never answers, caught mid-check.
        context = seeded(browser, ports, "silent", size)
        page = context.new_page()
        page.goto(url)
        page.get_by_role("tab", name="mock").click()
        page.get_by_text("Checking each hop").wait_for()
        expect(f"{tag} waiting state: six rows say Waiting and the button is off", page.locator('li[data-status="waiting"]').count() == 6 and page.get_by_role("button", name="Checking").is_disabled())
        shoot(page, size, SHOTS / f"drive-hops-state-waiting-{size[0]}.png")
        context.close()

        # Failed: the daemon refuses the device. By keyboard from here on.
        context = seeded(browser, ports, "refused-ownership", size)
        page = context.new_page()
        page.goto(url)
        page.get_by_role("tab", name="mock").click()
        page.wait_for_function(wait, timeout=30000)
        spill = page.evaluate("() => Math.max(0, ...[...document.querySelectorAll('.check, .check *')].map((e) => e.getBoundingClientRect().right)) - innerWidth")
        expect(f"{tag} failed state: nothing spills sideways", spill <= 0.5, spill)
        heights = page.eval_on_selector_all(".check .button", "(els) => els.map((e) => e.getBoundingClientRect().height)")
        expect(f"{tag} failed state: every button is 44px or taller", bool(heights) and min(heights) >= 44, heights)
        tree = page.locator(".check").aria_snapshot()
        (SHOTS / f"aria-failed-{size[0]}.txt").write_text(tree)
        expect(f"{tag} announced as: a status carrying the verdict, a list of six hops, and the copy button", 'status' in tree and "Broke at hop 6, Daemon admits this device." in tree and tree.count("listitem") == 6 and "Copy report for prod-director" in tree, tree[:300])
        page.get_by_role("button", name="Check again").focus()
        page.keyboard.press("Tab")
        reached = page.evaluate("document.activeElement.textContent.trim()")
        expect(f"{tag} keyboard: Tab from Check again lands on the copy button", reached.startswith("Copy report"), reached)
        hint = page.locator(".check .hint").inner_text()
        expect(f"{tag} an answered refusal does not point at Chrome's network error codes", "ERR_" not in hint and "reason" in hint, hint)
        page.keyboard.press("Enter")
        page.get_by_text("Copied. Paste it to prod-director.").wait_for()
        if browser.browser_type.name == "chromium":
            text = page.evaluate("navigator.clipboard.readText()")
            expect(f"{tag} copy: the clipboard holds a message to prod-director with the verdict and every hop", text.startswith("To prod-director:") and "Result: Broke at hop 6" in text and all(f"{i}. " in text for i in range(1, 7)), text[:200])
        shoot(page, size, SHOTS / f"drive-hops-state-failed-{size[0]}.png")
        context.close()

        # Copy refused: a page that may not write the clipboard still shows the report.
        context = seeded(browser, ports, "refused", size, "broken")
        page = context.new_page()
        page.goto(url)
        page.get_by_role("tab", name="mock").click()
        page.wait_for_function(wait, timeout=30000)
        page.get_by_role("button", name="Copy report for prod-director").click()
        box = page.get_by_label("Report for prod-director")
        box.wait_for()
        picked = page.evaluate("() => { const t = document.activeElement; return t.tagName === 'TEXTAREA' && t.selectionStart === 0 && t.selectionEnd === t.value.length && t.value.startsWith('To prod-director:'); }")
        expect(f"{tag} copy refused: the report is shown, focused and fully selected", picked)
        expect(f"{tag} copy refused: the status says to copy by hand", "copy it by hand" in page.locator(".check p.note").inner_text())
        shoot(page, size, SHOTS / f"drive-hops-state-copy-failed-{size[0]}.png")
        context.close()


def banner(browser, ports, url: str, expect) -> None:
    """A link that was up and drops offers the check from its own bar, a phone's one row included."""
    for size in [(1280, 800), (320, 640)]:
        tag = f"{size[0]}px"
        context = seeded(browser, ports, "flaps" if size[0] > 500 else "flaps-narrow", size)
        page = context.new_page()
        page.goto(url)
        page.get_by_role("tab", name="mock").click()
        ask = page.locator(".link").get_by_role("button", name="Check connection")
        ask.wait_for(timeout=40000)
        height = ask.bounding_box()["height"]
        expect(f"{tag} reconnecting: the bar offers Check connection, 44px or taller", height >= 44, height)
        spill = page.evaluate("() => document.querySelector('.link').scrollWidth - document.querySelector('.link').clientWidth")
        expect(f"{tag} reconnecting: the bar does not spill sideways", spill <= 0, spill)
        page.screenshot(path=str(SHOTS / f"drive-hops-state-banner-{size[0]}.png"))
        ask.click()
        page.wait_for_function("() => { const v = document.querySelector('.check p.verdict'); return v && v.textContent.trim() && !v.textContent.startsWith('Checking'); }", timeout=30000)
        said = page.locator(".check p.verdict").inner_text().strip()
        expect(f"{tag} reconnecting: the bar's button opens the host panel, and the check names the refused upgrade", said == "Broke at hop 5, Websocket upgrade." and page.get_by_role("heading", name="mock", level=1).count() == 1, said)
        context.close()


def main() -> int:
    SHOTS.mkdir(exist_ok=True)
    port = free_port()
    web, (daemon, ports) = serve_client(port), start_daemon()
    failures: list[str] = []

    def expect(label: str, ok: bool, detail: object = "") -> None:
        print(("pass " if ok else "FAIL ") + label + ("" if ok else f"  {detail}"))
        if not ok:
            failures.append(label)

    url = f"http://localhost:{port}/"
    try:
        with sync_playwright() as p:
            engine = getattr(p, os.environ.get("ENGINE", "chromium"))
            browser = engine.launch()
            print(f"engine {browser.browser_type.name} {browser.version}")
            for name, trusts, truth, owed in SCENARIOS:
                host = "no-such-name.invalid" if name == "unresolved" else "127.0.0.1"
                address = f"wss://{host}:{ports[name]}/"
                context = browser.new_context(viewport={"width": 1280, "height": 800}, ignore_https_errors=trusts)
                context.grant_permissions(["clipboard-read", "clipboard-write"]) if browser.browser_type.name == "chromium" else None
                context.add_init_script(f"localStorage.setItem('aterm.hosts.v1', JSON.stringify([{{label: 'mock', address: {json.dumps(address)}}}]))")
                page = context.new_page()
                failed: list[str] = []
                page.on("requestfailed", lambda r: failed.append(r.failure or "") if "/_aterm/reach" in r.url else None)
                page.goto(url)
                page.get_by_role("tab", name="mock").click()
                if name == "ok":
                    page.get_by_role("button", name="Check connection").click()
                verdict = page.locator("p.verdict")
                page.wait_for_function("() => { const v = document.querySelector('p.verdict'); return v && v.textContent.trim() && !v.textContent.startsWith('Checking'); }", timeout=30000)
                said = verdict.inner_text().strip()
                numbers = hop_numbers(page, "fail", "maybe")
                expect(f"{name}: the panel says '{owed}'", said == owed, said)
                if truth is not None:
                    expect(f"{name}: the hop that really broke ({truth}) is among those named {sorted(numbers)}", truth in numbers, numbers)
                    first = next((f for f in failed if f), "")
                    if first and re.search(NOT_A_HOP, first):
                        print(f"     {name}: the browser's code {first} is not a network failure, so it names no hop")
                    elif first:
                        mapped = next((hop for pattern, hop in NET_HOP if re.search(pattern, first)), None)
                        expect(f"{name}: the browser's own code {first} agrees it is hop {truth}", mapped == truth, (first, mapped))
                else:
                    expect(f"{name}: no hop is named", not numbers, numbers)
                page.locator(".check").screenshot(path=str(SHOTS / f"drive-hops-{name}-1280.png"))
                page.close()
                context.close()
            surface(browser, ports, url, expect)
            banner(browser, ports, url, expect)
            browser.close()
    finally:
        web.terminate()
        daemon.terminate()
    print(json.dumps({"failed": failures}))
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
