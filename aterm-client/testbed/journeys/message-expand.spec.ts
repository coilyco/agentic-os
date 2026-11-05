// COI-3362: a row in the Messages tab opens in place to the full text, and closes again.
// Opening only reads, so the waiting cue is untouched.
import { card, expect, kindOf, SEATS, test, type Bed } from "../harness.ts";

const LONG = Array.from({ length: 40 }, (_, i) => `Step ${i + 1}: run the gate, read what it says, and tell the person which step is next.`).join("\n");
const SHORT = "Run the gate.";

const message = (id: string, text: string) => ({
  type: "message",
  message: { id, from: "eng-platform Beetle-Ox", target: "frontend-eng", session: SEATS.imp.id, state: "delivered", text },
});

async function openMessages(bed: Bed, desktop: boolean) {
  const { page } = bed;
  if (desktop) {
    const toggle = page.getByRole("button", { name: /side panel/i }).first();
    if (await toggle.isVisible()) await toggle.click();
  }
  await page.getByRole("tab", { name: /^Messages/ }).click();
}

test("a long and a short message each open to their full text and close again", async ({ bed }, info) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.push(message("long-1", LONG));
  bed.mock.push(message("short-1", SHORT));
  // A question waiting on the person, so the waiting cue is there to be left alone.
  bed.mock.push({
    type: "ask",
    ask: { id: "wait-1", session: SEATS.imp.id, header: "Scope", question: "Ship it?", options: [{ label: "Yes", description: "Now." }, { label: "No", description: "Later." }], allow_other: false, multi: false },
  });
  await expect(card(bed.page)).toBeVisible();
  await openMessages(bed, kindOf(info) === "desktop");

  const rows = bed.page.getByRole("button", { name: /Beetle-Ox/ });
  await expect(rows).toHaveCount(2);
  const long = rows.nth(0);
  await expect(long, "collapsed").toHaveAttribute("aria-expanded", "false");
  await expect(long, "the first line shows before it is opened").toContainText("Step 1:");
  await bed.step("collapsed", { compare: false });

  await long.click();
  await expect(long).toHaveAttribute("aria-expanded", "true");
  const text = bed.page.getByRole("region", { name: /Full text from Beetle-Ox/ }).first();
  await expect(text).toBeVisible();
  await expect(text).toContainText("Step 40:");
  const box = await text.boundingBox();
  const viewport = bed.page.viewportSize()!;
  expect(box!.height, "a long message scrolls inside its row, not off the screen").toBeLessThan(viewport.height * 0.5);
  expect(box!.x + box!.width, "stays inside the screen").toBeLessThanOrEqual(viewport.width);
  expect(await text.evaluate((el) => el.scrollHeight > el.clientHeight), "the long text scrolls inside").toBe(true);
  await bed.step("expanded", { compare: false });

  await long.click();
  await expect(long).toHaveAttribute("aria-expanded", "false");
  await expect(text).toBeHidden();

  // The short one opens by keyboard.
  const short = rows.nth(1);
  await short.focus();
  await bed.page.keyboard.press("Enter");
  await expect(short).toHaveAttribute("aria-expanded", "true");
  await expect(bed.page.getByRole("region", { name: /Full text from Beetle-Ox/ })).toContainText(SHORT);
  await bed.page.keyboard.press("Space");
  await expect(short).toHaveAttribute("aria-expanded", "false");

  // Reading answered nothing and left the question waiting. A phone's sheet hides it.
  const hide = bed.page.getByRole("button", { name: "Hide the side panel" });
  if (await hide.isVisible()) await hide.click();
  await expect(card(bed.page), "opening a message cleared the waiting question").toBeVisible();
  expect(bed.mock.answers, "opening a message answered something").toEqual([]);
});

test("a host that sends no text says so instead of showing an empty row", async ({ bed }, info) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.push({ type: "message", message: { id: "old-1", from: "eng-platform Beetle-Ox", target: "frontend-eng", session: SEATS.imp.id, state: "delivered" } });
  await openMessages(bed, kindOf(info) === "desktop");
  const row = bed.page.getByRole("button", { name: /Beetle-Ox/ });
  await row.click();
  await expect(bed.page.getByText(/did not send the text of this message/)).toBeVisible();
});
