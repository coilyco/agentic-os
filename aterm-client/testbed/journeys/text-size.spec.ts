// COI-3481: a text size control for the terminal. A smaller size fits more columns in
// a phone, and the choice is the device's, so it outlives a reload.
import { card, expect, kindOf, quirk, SEATS, test, seatEntry } from "../harness.ts";
import { sceneFor } from "../scenes.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) === "landscape", "landscape is its own journey"));

// The button shows while the terminal is under 360px wide, the 34 columns of the record.
const wide = (info: import("@playwright/test").TestInfo) => info.project.name !== "phone-320";

const KEY = "aterm.terminal-text-size.v1";

const fontOf = (page: import("@playwright/test").Page) =>
  page.evaluate(() => getComputedStyle(document.querySelector(".xterm-rows")!).fontSize);

test("the size starts at 14px, and a smaller one fits more columns in the box", async ({ bed }, info) => {
  test.skip(wide(info), "the button shows only in a narrow terminal");
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, sceneFor("claude-fresh", {} as never));
  await expect(bed.screen()).toContainText("Hello. What should I work on?");
  await expect.poll(() => fontOf(bed.page)).toBe("14px");
  const before = bed.cols(SEATS.imp);
  const opener = bed.page.getByRole("button", { name: "Text size" });
  await expect(opener).toHaveAttribute("aria-expanded", "false");
  await opener.click();
  const smaller = bed.page.getByRole("button", { name: "Smaller text" });
  for (const target of [opener, smaller, bed.page.getByRole("button", { name: "Larger text" })]) {
    const box = (await target.boundingBox())!;
    expect(Math.min(box.width, box.height), "a target under 44px").toBeGreaterThanOrEqual(44);
  }
  await smaller.click();
  await smaller.click();
  await expect(bed.page.getByRole("group", { name: "Text size" })).toContainText("12px");
  await expect.poll(() => fontOf(bed.page)).toBe("12px");
  await expect.poll(() => bed.cols(SEATS.imp), { message: `columns asked for at 14px: ${before}` }).toBeGreaterThan(before);
  await bed.step("smaller", { compare: false });
});

test("the choice survives a reload, and the terminal comes back at it", async ({ bed }, info) => {
  test.skip(wide(info), "the button shows only in a narrow terminal");
  await bed.openSeat(SEATS.imp);
  await bed.page.getByRole("button", { name: "Text size" }).click();
  await bed.page.getByRole("button", { name: "Smaller text" }).click();
  await expect.poll(() => bed.page.evaluate((key) => localStorage.getItem(key), KEY)).toBe("13");
  const narrow = bed.cols(SEATS.imp);
  await bed.page.reload();
  await seatEntry(bed.page, SEATS.imp.name).click();
  await expect(bed.page.getByRole("heading", { name: SEATS.imp.name, level: 1 })).toBeAttached();
  await expect(bed.screen()).toBeVisible();
  await expect.poll(() => fontOf(bed.page)).toBe("13px");
  await expect.poll(() => bed.cols(SEATS.imp)).toBeGreaterThanOrEqual(narrow);
});

test("a wide box shows no button at 14px, and one at any other size so the way back stays", async ({ bed }, info) => {
  test.skip(!wide(info), "a narrow terminal always shows it");
  await bed.openSeat(SEATS.imp);
  await expect(bed.page.getByRole("button", { name: "Text size" })).toHaveCount(0);
  await bed.page.evaluate((key) => localStorage.setItem(key, "12"), KEY);
  await bed.page.reload();
  await seatEntry(bed.page, SEATS.imp.name).click();
  await expect(bed.page.getByRole("button", { name: "Text size" })).toBeVisible();
});

test("a stored size that is not a step is ignored", async ({ bed }) => {
  await bed.page.addInitScript((key) => localStorage.setItem(key, "15"), KEY);
  await bed.openSeat(SEATS.imp);
  await expect.poll(() => fontOf(bed.page)).toBe("14px");
});

test("by keyboard: open, step, and Escape closes", async ({ bed }, info) => {
  test.skip(wide(info), "the button shows only in a narrow terminal");
  await bed.openSeat(SEATS.imp);
  const opener = bed.page.getByRole("button", { name: "Text size" });
  await opener.focus();
  await bed.page.keyboard.press("Enter");
  await expect(opener).toHaveAttribute("aria-expanded", "true");
  const larger = bed.page.getByRole("button", { name: "Larger text" });
  await larger.focus();
  await bed.page.keyboard.press("Enter");
  await expect.poll(() => fontOf(bed.page)).toBe("16px");
  await bed.page.keyboard.press("Escape");
  await expect(opener).toHaveAttribute("aria-expanded", "false");
  await expect(bed.page.getByRole("group", { name: "Text size" })).toHaveCount(0);
});

// COI-3574: at 320x420 a card reaches the terminal's top, where the control sat on it.
// The control resizes the PTY, which a card must never do, so it waits for the card.
test("with a card open the size control steps aside and the card's own buttons are the ones tapped", quirk("COI-3574"), async ({ bed }, info) => {
  test.skip(wide(info), "the button shows only in a narrow terminal");
  await bed.openSeat(SEATS.imp);
  const opener = bed.page.getByRole("button", { name: "Text size" });
  await expect(opener).toBeVisible();
  await bed.page.setViewportSize({ width: 320, height: 420 });
  await expect.poll(() => bed.mock.idleMs(), { timeout: 15000 }).toBeGreaterThan(1500);
  bed.mock.push({
    type: "ask",
    ask: {
      id: "size-1",
      session: SEATS.imp.id,
      header: "Accent",
      question: "Which accent color should the client use for the active seat?",
      options: ["Purple", "Teal", "Black", "Gold"].map((label) => ({ label, description: "A description that wraps." })),
      allow_other: true,
      multi: false,
    },
  });
  await expect(card(bed.page)).toBeVisible();
  await expect(opener, "the size control is still over the card").toHaveCount(0);
  const fold = card(bed.page).getByRole("button", { name: "Hide question" });
  const box = (await fold.boundingBox())!;
  const topmost = await bed.page.evaluate(
    ({ x, y }) => document.elementFromPoint(x, y)?.closest("button")?.textContent?.trim() ?? null,
    { x: box.x + box.width - 6, y: box.y + box.height / 2 },
  );
  expect(topmost, "the right end of Hide question is covered by another control").toBe("Hide question");
  await bed.step("card-covers-size", { compare: false });
  await card(bed.page).getByRole("button", { name: "Cancel" }).click();
  await expect(card(bed.page)).toHaveCount(0);
  await expect(opener, "the size control does not come back after the card").toBeVisible();
});
