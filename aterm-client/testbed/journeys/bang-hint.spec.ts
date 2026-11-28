// COI-3802: a line starting with "!" says where it runs, in the seat composer and in the
// shell beside the seat, and goes as soon as the line stops starting with "!".
import type { Scene } from "../scenes.ts";
import { expect, kindOf, SEATS, test, type Bed } from "../harness.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) === "landscape", "the composer and shell at phone and desktop widths"));

const SHELL = "Runs in a plain terminal. No agent sees it.";

const bubble = (bed: Bed, inside: string) => bed.page.locator(`${inside} .bubble`);

/** The hint stays on screen, never wider than the window. */
async function onScreen(bed: Bed, inside: string, part = " .bubble"): Promise<void> {
  const box = (await bed.page.locator(`${inside}${part}`).boundingBox())!;
  const width = bed.page.viewportSize()!.width;
  expect(box.x, "starts on screen").toBeGreaterThanOrEqual(0);
  expect(box.x + box.width, "ends on screen").toBeLessThanOrEqual(width);
}

/** The seat's PTY size once it has held still, since a seat settles after it opens. */
async function settled(bed: Bed, seat: string): Promise<{ rows: number; cols: number }> {
  let last = "";
  await expect
    .poll(async () => {
      const now = JSON.stringify(bed.mock.boxOf(seat) ?? null);
      const same = now === last && now !== "null";
      last = now;
      if (!same) await bed.page.waitForTimeout(500);
      return same;
    }, { message: "the PTY size holds still" })
    .toBe(true);
  return bed.mock.boxOf(seat)!;
}

test("the seat composer says a ! line runs in the seat's shell, says it once, and never resizes the seat", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const field = bed.page.locator("#composer-input");
  const status = bed.page.locator("#composer-bang");
  await field.click();
  const before = await settled(bed, SEATS.imp.id);
  await expect(status, "the status region is there before it has words").toHaveCount(1);

  await bed.page.keyboard.type("!");
  const line = "Runs in Imp-Dragonfly's shell. The seat sees the output.";
  await expect(bubble(bed, ".field")).toHaveText(line);
  await expect(status, "announced from a region mounted all along").toHaveText(line);
  await expect(field, "and read with the field").toHaveAttribute("aria-describedby", /composer-bang/);
  await onScreen(bed, ".field");
  await bed.page.keyboard.type(" ls");
  await bed.step("seat-hint");
  await bed.page.waitForTimeout(400);
  expect(bed.mock.boxOf(SEATS.imp.id), "the hint floats, so the PTY keeps its size").toEqual(before);

  await field.fill(" ls");
  await expect(bubble(bed, ".field"), "gone once the line no longer starts with !").toHaveCount(0);
  await expect(status).toHaveText("");

  await field.fill("! ls");
  await expect(bubble(bed, ".field")).toBeVisible();
  const send = bed.page.locator(".composer button[type=submit]");
  await expect(send, "it never blocks sending").toBeEnabled();
  await send.click();
  await expect.poll(() => bed.mock.typed(SEATS.imp.id).join(""), { message: "the line went to the seat" }).toContain("! ls");
  await expect(bubble(bed, ".field"), "an empty field has no hint").toHaveCount(0);
});

test("a seat whose harness has no ! shell mode says the line goes as a message", async ({ bed }) => {
  bed.mock.patchSession(SEATS.beetle.id, { seat: "goose" });
  await bed.openSeat(SEATS.beetle);
  await bed.page.locator("#composer-input").click();
  await bed.page.keyboard.type("!pwd");
  await expect(bubble(bed, ".field")).toHaveText("Sent to Beetle-Ox as a message. goose has no ! shell mode.");
});

// A plain shell: it echoes what it is typed, as a shell's line editor does.
const shell = (): Scene => {
  let text = "";
  return {
    render: () => [`$ ${text}`],
    input: (data) => {
      text = /[\r\n\x03]/.test(data) ? "" : text + data.replace(/\x7f/g, "");
      return true;
    },
  };
};

test("the shell beside the seat says a ! line runs in a plain terminal, and only while it starts with !", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  bed.mock.addTerminal(SEATS.griffin.id, shell());
  const tab = bed.page.locator("#side-tab-terminal");
  if (!(await tab.isVisible())) await bed.page.getByRole("button", { name: /Side panel|Show panel/ }).first().click();
  await tab.click();
  const input = bed.page.locator(".pane .xterm-helper-textarea");
  await bed.page.locator(".pane .xterm").click();
  await expect(input).toBeFocused();
  const status = bed.page.locator(`#bang-${SEATS.griffin.id}`);
  const barHeight = (await bed.page.locator(".pane .bar").boundingBox())!.height;

  await bed.page.keyboard.type("! ls");
  await expect(bed.page.locator(".pane .bar .bang"), "the shell's label says it").toHaveText(SHELL);
  await expect(bed.page.locator(".pane .bar"), "in place of its name").not.toContainText("Shell beside");
  await expect(status, "announced, with focus left in the shell").toHaveText(SHELL);
  await expect(input).toBeFocused();
  const bar = bed.page.locator(".pane .bar");
  await onScreen(bed, ".pane .bar .bang", "");
  expect((await bar.boundingBox())!.height, "in the bar's own height, so the shell keeps its size").toBe(barHeight);
  await bed.step("shell-hint", { mask: [bed.page.locator(".pane .term-host")] });

  for (let i = 0; i < 4; i++) await bed.page.keyboard.press("Backspace");
  await expect(bed.page.locator(".pane .bar .bang"), "gone with the !").toHaveCount(0);
  await expect(bed.page.locator(".pane .bar"), "and the name is back").toContainText("Shell beside Griffin-Goose");
  await bed.page.keyboard.type("ls");
  await expect(bed.page.locator(".pane .bar .bang"), "a line that does not start with ! has none").toHaveCount(0);
  await bed.page.keyboard.press("Enter");

  await bed.page.keyboard.type("!");
  await expect(bed.page.locator(".pane .bar .bang")).toBeVisible();
  await bed.page.keyboard.press("Enter");
  await expect(bed.page.locator(".pane .bar .bang"), "a run line starts a fresh one").toHaveCount(0);

  // A recalled line is the shell's, not typed, so the hint stays off, not a guess.
  await bed.page.keyboard.press("ArrowUp");
  await bed.page.keyboard.type("!");
  await expect(bed.page.locator(".pane .bar .bang")).toHaveCount(0);
});
