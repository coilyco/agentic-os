// COI-3544: a switch wraps a seat's long lines to a phone's width. Panning stays default.
// The client alone draws it, so a desktop view of the seat is not asked to change.
import { expect, kindOf, SEATS, test, seatEntry } from "../harness.ts";
import type { Scene } from "../scenes.ts";

const TAIL = "END-OF-THE-LONG-LINE";
const WIDE = 120;

// A seat whose newest line is wider than any phone, with its input row below it.
const wideScene: Scene = {
  render(cols, rows) {
    const long = `⏺ The command that was run, with every flag spelled out, ${TAIL}`.slice(0, cols);
    const tail = [long, "", "❯ ", "─".repeat(cols)];
    return [...Array(Math.max(0, rows - tail.length)).fill(""), ...tail];
  },
};

const reading = (page: import("@playwright/test").Page) =>
  page.evaluate((tail) => {
    const host = document.querySelector<HTMLElement>(".term-host")!;
    const row = [...document.querySelectorAll<HTMLElement>(".xterm-rows > div")].find((r) => r.textContent?.includes(tail));
    const frame = host.getBoundingClientRect();
    const rects = [...(row?.getClientRects() ?? [])];
    const lines = new Set([...(row?.querySelectorAll("span") ?? [])].flatMap((span) => [...span.getClientRects()].map((rect) => Math.round(rect.top)))).size;
    return {
      overflow: host.scrollWidth - host.clientWidth,
      hostLeft: frame.left,
      hostRight: frame.right,
      rowRight: Math.max(0, ...rects.map((rect) => rect.right)),
      lines,
      wrapping: host.classList.contains("wrapping"),
    };
  }, TAIL);

// These journeys press the switch themselves, so the default starts off (COI-3612).
const noDefault = (page: import("@playwright/test").Page) => page.addInitScript(() => localStorage.setItem("aterm.terminal-wrap-default.v1", "off"));

test.describe("on a phone", () => {
  test.beforeEach(async ({ page }, info) => {
    test.skip(kindOf(info) !== "phone", "a phone's narrow box");
    await noDefault(page);
  });

  test("the switch wraps the long line to the screen and one tap undoes it", async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    bed.mock.holdWide(SEATS.imp.id, WIDE);
    bed.mock.setScene(SEATS.imp.id, wideScene);
    await expect(bed.screen()).toContainText(TAIL);
    const wrap = bed.page.getByRole("button", { name: "Wrap long lines" });
    await expect(wrap, "off until a person turns it on").toHaveAttribute("aria-pressed", "false");
    const box = (await wrap.boundingBox())!;
    expect(box.width, "a 44px target").toBeGreaterThanOrEqual(44);
    expect(box.height, "a 44px target").toBeGreaterThanOrEqual(44);
    const cut = await reading(bed.page);
    expect(cut.overflow, "sideways panning stays the default").toBeGreaterThan(100);
    await bed.step("off", { compare: false });

    const heard = bed.mock.heard.length;
    await wrap.click();
    await expect(wrap).toHaveAttribute("aria-pressed", "true");
    await expect.poll(async () => (await reading(bed.page)).overflow, "no text lies past the edge").toBeLessThanOrEqual(1);
    const wrapped = await reading(bed.page);
    expect(wrapped.rowRight, "the last character ends inside the box").toBeLessThanOrEqual(wrapped.hostRight + 1);
    expect(wrapped.lines, "the line folded onto more than one row").toBeGreaterThan(1);
    await expect(bed.page.locator(".edge.right"), "no sideways cue while lines fold").not.toHaveClass(/\bon\b/);
    await bed.step("wrapped", { compare: false });
    await expect.poll(() => bed.mock.idleMs()).toBeGreaterThan(500);
    expect(bed.mock.heard.slice(heard), "the switch sends the daemon nothing, so no other view moves").toEqual([]);

    await wrap.click();
    await expect(wrap).toHaveAttribute("aria-pressed", "false");
    await expect.poll(async () => (await reading(bed.page)).overflow, "panning is back").toBeGreaterThan(100);
    expect((await reading(bed.page)).lines, "the line is one row again").toBe(1);
    await bed.step("off-again", { compare: false });
  });

  test("the switch is remembered for that seat on this device, and works by keyboard", async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    bed.mock.holdWide(SEATS.imp.id, WIDE);
    bed.mock.setScene(SEATS.imp.id, wideScene);
    await expect(bed.screen()).toContainText(TAIL);
    const wrap = bed.page.getByRole("button", { name: "Wrap long lines" });
    await wrap.focus();
    await bed.page.keyboard.press("Enter");
    await expect(wrap).toHaveAttribute("aria-pressed", "true");
    await bed.page.reload();
    await seatEntry(bed.page, SEATS.imp.name).click();
    await expect(bed.page.getByRole("heading", { name: SEATS.imp.name, level: 1 })).toBeAttached();
    bed.mock.holdWide(SEATS.imp.id, WIDE);
    bed.mock.setScene(SEATS.imp.id, wideScene);
    await expect(bed.screen()).toContainText(TAIL);
    await expect(bed.page.getByRole("button", { name: "Wrap long lines" }), "still on after a reload").toHaveAttribute("aria-pressed", "true");
    expect((await reading(bed.page)).wrapping).toBe(true);
    await bed.switchSeat(SEATS.beetle);
    bed.mock.holdWide(SEATS.beetle.id, WIDE);
    await expect(bed.page.getByRole("button", { name: "Wrap long lines" }), "another seat is its own").toHaveAttribute("aria-pressed", "false");
  });

  test("typing and vertical scrolling work while lines are wrapped", async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    bed.mock.holdWide(SEATS.imp.id, WIDE);
    bed.mock.setScene(SEATS.imp.id, wideScene);
    await expect(bed.screen()).toContainText(TAIL);
    await bed.page.getByRole("button", { name: "Wrap long lines" }).click();
    await expect.poll(async () => (await reading(bed.page)).wrapping).toBe(true);
    await bed.page.locator(".xterm-helper-textarea").focus();
    await bed.page.keyboard.type("ok");
    await expect.poll(() => bed.mock.typed(SEATS.imp.id).join(""), "keys reach the seat").toContain("ok");
    // The newest rows stay in view as the seat writes.
    for (let line = 0; line < 30; line++) bed.mock.push({ type: "output", session: SEATS.imp.id, data: Buffer.from(`\r\nline ${line}`).toString("base64") });
    await expect(bed.screen()).toContainText("line 29");
    const inView = await bed.page.evaluate(() => {
      const host = document.querySelector<HTMLElement>(".term-host")!;
      const rows = [...document.querySelectorAll<HTMLElement>(".xterm-rows > div")].filter((r) => r.textContent?.includes("line 29"));
      const at = rows.at(-1)?.getBoundingClientRect();
      const frame = host.getBoundingClientRect();
      return Boolean(at && at.bottom <= frame.bottom + 1 && at.top >= frame.top);
    });
    expect(inView, "the newest line is inside the box").toBe(true);
    await bed.step("typing", { compare: false });
  });
});

test.describe("on a phone, links and stripes", () => {
  test.beforeEach(async ({ page }, info) => {
    test.skip(kindOf(info) !== "phone", "a phone's narrow box");
    await noDefault(page);
  });

  const LINK = "https://code.claude.com/docs/en/permission-modes-and-the-rest-of-it";
  const linkScene: Scene = {
    render(cols, rows) {
      const tail = [`see ${LINK} for the modes`.slice(0, cols), "", "❯ "];
      return [...Array(Math.max(0, rows - tail.length)).fill(""), ...tail];
    },
  };

  // The middle of a piece of text, where a finger would land, in page pixels.
  const centreOf = (page: import("@playwright/test").Page, piece: string) =>
    page.evaluate((needle) => {
      const walker = document.createTreeWalker(document.querySelector(".xterm-rows")!, NodeFilter.SHOW_TEXT);
      for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        const at = node.textContent?.indexOf(needle) ?? -1;
        if (at < 0) continue;
        const range = document.createRange();
        range.setStart(node, at);
        range.setEnd(node, at + needle.length);
        const box = range.getClientRects()[0]!;
        return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
      }
      return null;
    }, piece);

  async function openFolded(bed: import("../harness.ts").Bed) {
    await bed.openSeat(SEATS.imp);
    await bed.page.evaluate(() => {
      const w = window as unknown as { __opened: string[] };
      w.__opened = [];
      window.open = (url) => (w.__opened.push(String(url)), null);
    });
    bed.mock.holdWide(SEATS.imp.id, WIDE);
    bed.mock.setScene(SEATS.imp.id, linkScene);
    await expect(bed.screen()).toContainText("permission-modes");
    await bed.page.getByRole("button", { name: "Wrap long lines" }).click();
    await expect.poll(async () => (await reading(bed.page)).wrapping).toBe(true);
    await bed.page.waitForTimeout(500);
  }
  const opened = (page: import("@playwright/test").Page) => page.evaluate(() => (window as unknown as { __opened: string[] }).__opened);

  test("a tap on a link in a folded row opens it, from either part of the address", async ({ bed }) => {
    await openFolded(bed);
    const rest = await centreOf(bed.page, "the-rest-of-it");
    expect(rest, "the tail of the address is on screen").not.toBeNull();
    const head = await centreOf(bed.page, "code.claude");
    expect(head!.y, "the address folds onto more than one visual line").toBeLessThan(rest!.y);
    await bed.page.touchscreen.tap(rest!.x, rest!.y);
    await expect.poll(() => opened(bed.page)).toEqual([LINK]);
    await bed.page.touchscreen.tap(head!.x, head!.y);
    await expect.poll(() => opened(bed.page)).toEqual([LINK, LINK]);
    await bed.step("link-folded", { compare: false });
  });

  test("a tap on text that is not a link opens nothing", async ({ bed }) => {
    await openFolded(bed);
    const word = await centreOf(bed.page, "the modes");
    expect(word, "the words after the address are on screen").not.toBeNull();
    await bed.page.touchscreen.tap(word!.x, word!.y);
    await bed.page.waitForTimeout(400);
    expect(await opened(bed.page)).toEqual([]);
  });

  test("an envelope's stripe runs down the whole folded row, and goes with the switch", async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    bed.mock.push({
      type: "message",
      message: { id: "env-1", from: "eng-platform Beetle-Ox", target: "frontend-eng", session: SEATS.imp.id, state: "delivered", text: "Run the gate." },
    });
    const envelope = "[from eng-platform Beetle-Ox] Run the gate, read what it says, and tell the person which step is next.";
    bed.mock.holdWide(SEATS.imp.id, WIDE);
    bed.mock.setScene(SEATS.imp.id, { render: (cols, rows) => [...Array(Math.max(0, rows - 3)).fill(""), envelope.slice(0, cols), "", "❯ "] });
    await expect(bed.screen()).toContainText("tell the person");
    const stripe = () =>
      bed.page.evaluate(() => {
        const row = [...document.querySelectorAll<HTMLElement>(".xterm-rows > div")].find((r) => r.textContent?.includes("[from eng-platform"));
        if (!row) return null;
        const bar = getComputedStyle(row, "::before");
        return { marked: "stripe" in row.dataset, width: bar.width, tall: row.getBoundingClientRect().height, background: row.style.background };
      });
    await bed.page.getByRole("button", { name: "Wrap long lines" }).click();
    await expect.poll(async () => (await stripe())?.marked, "the row carries a stripe").toBe(true);
    const on = (await stripe())!;
    expect(on.width, "a 4px bar").toBe("4px");
    expect(on.tall, "the bar spans every folded line of the row").toBeGreaterThan(40);
    expect(on.background).toContain("color-mix");
    await bed.step("stripe-folded", { compare: false });
    await bed.page.getByRole("button", { name: "Wrap long lines" }).click();
    await expect.poll(async () => (await stripe())?.marked, "the stripe leaves with the switch").toBe(false);
  });
});

test.describe("on a desktop", () => {
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "a desktop pane"));

  test("there is no switch, since a pane scrolls its long lines", async ({ bed }) => {
    await bed.openSeat(SEATS.imp);
    bed.mock.holdWide(SEATS.imp.id, 340);
    bed.mock.setScene(SEATS.imp.id, wideScene);
    await expect(bed.screen()).toContainText(TAIL);
    await expect(bed.page.getByRole("button", { name: "Wrap long lines" })).toHaveCount(0);
  });

  test("a desktop view does not change while a phone of the same seat wraps", async ({ bed, browser, baseURL }) => {
    await bed.openSeat(SEATS.imp);
    bed.mock.holdWide(SEATS.imp.id, WIDE);
    bed.mock.setScene(SEATS.imp.id, wideScene);
    await expect(bed.screen()).toContainText(TAIL);
    await bed.step("desktop-before", { compare: false });
    const before = await bed.page.screenshot();

    // A phone attaches to the same seat. Its Auto default folds the line with no tap.
    const phone = await browser.newContext({ viewport: { width: 412, height: 915 }, hasTouch: true, isMobile: true, baseURL });
    const page = await phone.newPage();
    await page.goto("/");
    await page.getByRole("tab", { name: /this Mac/ }).click();
    await seatEntry(page, SEATS.imp.name).click();
    await expect.poll(async () => (await reading(page)).wrapping).toBe(true);
    await page.waitForTimeout(1000);
    expect((await reading(page)).overflow, "the phone's text lies inside its box").toBeLessThanOrEqual(1);
    await page.screenshot({ path: test.info().outputPath("phone-wrapped.png") });

    await expect.poll(() => bed.mock.idleMs()).toBeGreaterThan(500);
    await bed.step("desktop-while-phone-wraps", { compare: false });
    const after = await bed.page.screenshot();
    expect(after.equals(before), "the desktop's picture is the same bytes with the phone wrapping").toBe(true);
    await phone.close();
  });
});

// COI-3612: Settings holds a device default, and a seat's own button overrides it.
test.describe("the device default on a phone", () => {
  test.use({ wrapDefault: "auto" });
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "a phone's narrow box"));

  const open = async (bed: import("../harness.ts").Bed, seat = SEATS.imp) => {
    await bed.openSeat(seat);
    bed.mock.holdWide(seat.id, WIDE);
    bed.mock.setScene(seat.id, wideScene);
    await expect(bed.screen()).toContainText(TAIL);
  };
  const button = (bed: import("../harness.ts").Bed) => bed.page.getByRole("button", { name: "Wrap long lines" });

  test("Auto folds a seat that runs past the screen with no tap, and the button turns it off for good", async ({ bed }) => {
    await open(bed);
    await expect(button(bed), "folded by the default").toHaveAttribute("aria-pressed", "true");
    await expect.poll(async () => (await reading(bed.page)).overflow, "no text lies past the edge").toBeLessThanOrEqual(1);
    await button(bed).click();
    await expect(button(bed)).toHaveAttribute("aria-pressed", "false");
    await expect.poll(async () => (await reading(bed.page)).overflow, "panning is back").toBeGreaterThan(100);
    await bed.page.reload();
    await seatEntry(bed.page, SEATS.imp.name).click();
    bed.mock.holdWide(SEATS.imp.id, WIDE);
    bed.mock.setScene(SEATS.imp.id, wideScene);
    await expect(bed.screen()).toContainText(TAIL);
    await expect(button(bed), "the seat's own choice beats the default after a reload").toHaveAttribute("aria-pressed", "false");
    await bed.switchSeat(SEATS.beetle);
    bed.mock.holdWide(SEATS.beetle.id, WIDE);
    bed.mock.setScene(SEATS.beetle.id, wideScene);
    await expect(button(bed), "another seat still follows the default").toHaveAttribute("aria-pressed", "true");
  });

  test("Off leaves a seat panning until its button is pressed", async ({ bed }) => {
    await bed.page.addInitScript(() => localStorage.setItem("aterm.terminal-wrap-default.v1", "off"));
    await open(bed);
    await expect(button(bed)).toHaveAttribute("aria-pressed", "false");
    expect((await reading(bed.page)).overflow, "sideways panning").toBeGreaterThan(100);
  });
});
