// COI-3481: a text size control for the terminal. A smaller size fits more columns in
// a phone, and the choice is the device's, so it outlives a reload.
import { expect, kindOf, SEATS, test } from "../harness.ts";
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
  await bed.page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first().click();
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
  await bed.page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first().click();
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
