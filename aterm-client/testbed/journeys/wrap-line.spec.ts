// COI-3544: a switch wraps a seat's long lines to a phone's width. Panning stays default.
// The client alone draws it, so a desktop view of the seat is not asked to change.
import { expect, kindOf, SEATS, test } from "../harness.ts";
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

test.describe("on a phone", () => {
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "a phone's narrow box"));

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

    // A phone attaches to the same seat and turns the switch on.
    const phone = await browser.newContext({ viewport: { width: 412, height: 915 }, hasTouch: true, isMobile: true, baseURL });
    const page = await phone.newPage();
    await page.goto("/");
    await page.getByRole("tab", { name: /this Mac/ }).click();
    await page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first().click();
    await page.getByRole("button", { name: "Wrap long lines" }).click();
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
