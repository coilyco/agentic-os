// COI-3690: a sent message shows in the transcript on the next frame, marked pending, and
// becomes the real entry with no copy beside it. One the seat never takes says so.
import { expect, kindOf, pushOutput, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { transcript: true } });

const imp = SEATS.imp.id;
const history = [
  { id: "p1", kind: "prompt", text: "Why is the stripe wrong?", at: "2026-11-10T21:00:00Z" },
  { id: "r1", kind: "reply", text: "A folded row breaks it.", complete: true, at: "2026-11-10T21:00:02Z" },
];
const sendingRow = (bed: Bed) => bed.page.locator('[data-row^="sending-"]');
const mine = (bed: Bed, words: string) => bed.page.locator(".transcript .turn.me").filter({ hasText: words });

async function openTranscript(bed: Bed): Promise<void> {
  bed.mock.setTranscript(imp, history);
  await bed.openSeat(SEATS.imp);
  if (kindOf(test.info()) === "desktop") await bed.page.getByRole("radio", { name: "Focus" }).click();
  await expect(bed.page.locator(".transcript")).toContainText("A folded row breaks it.");
}

/** Presses Send in the page and says if the entry is on screen by the next frame. */
const sendAndLook = (bed: Bed, words: string) =>
  bed.page.evaluate(async (typed) => {
    const field = document.querySelector<HTMLTextAreaElement>("#composer-input")!;
    field.value = typed;
    field.dispatchEvent(new Event("input", { bubbles: true }));
    await new Promise((done) => setTimeout(done, 50));
    const start = performance.now();
    document.querySelector<HTMLButtonElement>("form.composer button.primary")!.click();
    return new Promise<{ shown: boolean; ms: number }>((done) =>
      requestAnimationFrame(() => done({ shown: document.querySelector('[data-row^="sending-"]')?.textContent?.includes(typed) === true, ms: performance.now() - start })),
    );
  }, words);

test("a sent message is on screen by the next frame, then is the real entry with no copy", async ({ bed }) => {
  await openTranscript(bed);
  const seen = await sendAndLook(bed, "ship the stripe fix");
  expect(seen.shown, "the message was not in the transcript one frame after Send").toBe(true);
  await expect(sendingRow(bed)).toContainText("Sending.");
  await bed.step("pending", { compare: false });

  // The harness writes it. The next read carries it, and the entry goes with it.
  bed.mock.appendTranscript(imp, [{ id: "p2", kind: "prompt", text: "ship the stripe fix", at: new Date().toISOString() }]);
  await expect(sendingRow(bed), "the pending entry stayed after the real one came").toHaveCount(0, { timeout: 6000 });
  await expect(mine(bed, "ship the stripe fix"), "the message shows once").toHaveCount(1);
  await expect(bed.page.locator('[data-row="p2"]')).toHaveCount(1);
});

test("a message the seat never takes stays visibly unsent, and a late prompt still settles it", async ({ bed }) => {
  await bed.page.clock.install();
  await openTranscript(bed);
  await sendAndLook(bed, "did you get this");
  await expect(sendingRow(bed)).toHaveAttribute("data-state", "pending");

  await bed.page.clock.fastForward(31_000);
  await expect(sendingRow(bed)).toHaveAttribute("data-state", "unsent");
  await expect(sendingRow(bed).getByRole("alert")).toContainText("Not in the transcript yet");
  await expect(sendingRow(bed).getByRole("button", { name: "Dismiss" })).toBeVisible();
  await bed.step("unsent", { compare: false });

  bed.mock.appendTranscript(imp, [{ id: "p3", kind: "prompt", text: "did you get this", at: new Date().toISOString() }]);
  await bed.page.clock.runFor(3000);
  await expect(sendingRow(bed)).toHaveCount(0);
  await expect(mine(bed, "did you get this")).toHaveCount(1);
});

test("an unsent entry can be dismissed", async ({ bed }) => {
  await bed.page.clock.install();
  await openTranscript(bed);
  await sendAndLook(bed, "swallowed by a dialog");
  await bed.page.clock.fastForward(31_000);
  await sendingRow(bed).getByRole("button", { name: "Dismiss" }).click();
  await expect(sendingRow(bed)).toHaveCount(0);
});

// COI-3690 gate: taken while the view was closed, then buried under the newest page.
// A typed turn and a mid-turn queued_command both reach the client as a prompt.
const stamp = (n: number) => new Date(Date.now() + n).toISOString();
const busyWork = (from: number, count: number, last: string) => [
  ...Array.from({ length: count }, (_, n) => ({ id: `t${from + n}`, kind: "tool", at: stamp(from + n), tool: { name: "Bash", class: "shell", summary: `Ran step ${from + n}`, ok: true } })),
  { id: `r${from}`, kind: "reply", text: last, complete: true, at: stamp(from + count) },
];

/** Opens another seat in the app, so the view closes and the entry stays in memory. */
async function away(bed: Bed): Promise<void> {
  await bed.switchSeat(SEATS.beetle);
  await expect(bed.page.locator(".session h1")).toContainText(SEATS.beetle.name);
}

async function back(bed: Bed, last: string): Promise<void> {
  await bed.switchSeat(SEATS.imp);
  if (kindOf(test.info()) === "desktop") await bed.page.getByRole("radio", { name: "Focus" }).click();
  await expect(bed.page.locator(".transcript")).toContainText(last);
}

test("a plain turn deep under the newest page still settles its pending entry", async ({ bed }) => {
  await openTranscript(bed);
  await sendAndLook(bed, "should be connected now");
  await expect(sendingRow(bed)).toHaveCount(1);
  await away(bed);
  bed.mock.appendTranscript(imp, [{ id: "deep1", kind: "prompt", text: "should be connected now", at: stamp(0) }, ...busyWork(100, 150, "Connected and checked.")]);
  await back(bed, "Connected and checked.");
  await expect(sendingRow(bed), "a prompt older than the newest page left its pending entry").toHaveCount(0, { timeout: 6000 });
  // The real entry is above the page on screen, so nothing shows twice.
  await expect(mine(bed, "should be connected now")).toHaveCount(0);
  expect(bed.mock.transcriptCalls.some((call) => call.session === imp && call.before !== undefined), "the view read back to find it").toBe(true);
});

test("a message absorbed mid-turn settles, sitting between tool calls deep in the transcript", async ({ bed }) => {
  await openTranscript(bed);
  await sendAndLook(bed, "and pick up the other work in flight");
  await away(bed);
  bed.mock.appendTranscript(imp, [...busyWork(300, 20, "Still on the first task."), { id: "queued1", kind: "prompt", text: "and pick up the other work in flight", at: stamp(330) }, ...busyWork(400, 120, "Picked up the rest.")]);
  await back(bed, "Picked up the rest.");
  await expect(sendingRow(bed), "a mid-turn message left its pending entry").toHaveCount(0, { timeout: 6000 });
});

test("a message the seat never took stays unsent after the view reads back for it", async ({ bed }) => {
  await bed.page.clock.install();
  await openTranscript(bed);
  await sendAndLook(bed, "lost in a dialog");
  await away(bed);
  bed.mock.appendTranscript(imp, busyWork(600, 120, "Nothing came in."));
  await back(bed, "Nothing came in.");
  await bed.page.clock.fastForward(31_000);
  await expect(sendingRow(bed)).toHaveAttribute("data-state", "unsent");
  await expect(sendingRow(bed).getByRole("alert")).toContainText("Not in the transcript yet");
});

test("a message sent while the seat works reads as queued, never asks to be dismissed, and settles when taken", async ({ bed }) => {
  await bed.page.clock.install();
  await openTranscript(bed);
  // Working is recent output, so keep it coming while the seat should read busy.
  const pump = setInterval(() => pushOutput(bed.mock, imp, "."), 300);
  try {
    await expect(bed.page.locator('.field[data-working="true"]')).toHaveCount(1);
    await sendAndLook(bed, "and pick up the rest");
    await bed.page.clock.fastForward(31_000);
    await expect(sendingRow(bed)).toHaveAttribute("data-state", "queued");
    await expect(sendingRow(bed)).toContainText("Queued.");
    await expect(sendingRow(bed).getByRole("button", { name: "Dismiss" }), "a message the seat holds asked to be dismissed").toHaveCount(0);
    await bed.step("queued", { compare: false });
    bed.mock.appendTranscript(imp, [{ id: "absorbed", kind: "prompt", text: "and pick up the rest", at: new Date().toISOString() }]);
    await bed.page.clock.runFor(3000);
    await expect(sendingRow(bed)).toHaveCount(0);
  } finally {
    clearInterval(pump);
  }
});

test("a queued message the seat stops without turns unsent once the seat is quiet", async ({ bed }) => {
  await bed.page.clock.install();
  await openTranscript(bed);
  const pump = setInterval(() => pushOutput(bed.mock, imp, "."), 300);
  try {
    await expect(bed.page.locator('.field[data-working="true"]')).toHaveCount(1);
    await sendAndLook(bed, "swallowed at the end of a turn");
    await bed.page.clock.fastForward(31_000);
    await expect(sendingRow(bed)).toHaveAttribute("data-state", "queued");
  } finally {
    clearInterval(pump);
  }
  await expect(bed.page.locator('.field[data-working="true"]')).toHaveCount(0, { timeout: 8000 });
  await bed.page.clock.runFor(6000);
  await expect(sendingRow(bed)).toHaveAttribute("data-state", "unsent");
  await expect(sendingRow(bed).getByRole("button", { name: "Dismiss" })).toBeVisible();
});
