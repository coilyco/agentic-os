// COI-3532: Focus, Default and Verbose views of a seat, read from its transcript.
// Peer messages never show in them. The raw screen stays under them and in Verbose.
import { existsSync, readFileSync } from "node:fs";
import { expect, kindOf, SEATS, test, type Bed, reopenSeat, pickView, viewButton } from "../harness.ts";

test.use({ mockOptions: { transcript: true } });

const imp = SEATS.imp.id;
const OUTPUT = Array.from({ length: 8 }, (_, i) => `gate line ${i + 1}`).join("\n");

const transcript = [
  { id: "p1", kind: "prompt", text: "Why does the wrapped row hide the stripe?", at: "2026-11-10T21:00:00Z" },
  { id: "r1", kind: "reply", text: "I will read the component first.", at: "2026-11-10T21:00:02Z" },
  { id: "t1", kind: "tool", tool: { name: "Read", class: "read", summary: "Read Terminal.svelte", output: "654 lines", ok: true } },
  { id: "t2", kind: "tool", tool: { name: "Bash", class: "shell", summary: "Ran just aterm-client-gate", output: OUTPUT, ok: false } },
  { id: "r2", kind: "reply", text: "The stripe was placed by row number, which a folded row breaks.", complete: true, model: "claude-opus" },
  { id: "x1", kind: "peer", text: "[from eng-platform Beetle-Ox] Is the frame fixed?", from: { role: "eng-platform", identity: "Beetle-Ox" } },
  { id: "x3", kind: "tool", tool: { name: "Bash", class: "shell", summary: "Ran git log" }, for: "x1" },
  { id: "x2", kind: "reply", text: "Yes, the frame is fixed.", complete: true, for: "x1" },
  { id: "p2", kind: "prompt", text: "Thanks, carry on." },
  { id: "r3", kind: "reply", text: "Carrying on.", complete: true },
];

const view = (bed: Bed) => bed.page.locator(".transcript");

async function open(bed: Bed) {
  bed.mock.setTranscript(imp, transcript);
  await bed.openSeat(SEATS.imp);
}

test.describe("on a phone", () => {
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "a phone's default view"));

  test.describe("with Send feedback in the bar", () => {
    test.use({ mockOptions: { feedback: true, version: "aos-v0.578.0" } });

    test("one tap on View shows all five as 44px targets, arrows move between them, and Escape goes back", async ({ bed }, info) => {
      await open(bed);
      const button = viewButton(bed.page);
      const feedback = (await bed.page.getByRole("button", { name: "Send feedback", exact: true }).boundingBox())!;
      const own = (await button.boundingBox())!;
      expect(own.height, "as tall as Send feedback beside it").toBe(feedback.height);
      expect(own.x + own.width, "to its left, beside the seat picker").toBeLessThanOrEqual(feedback.x);
      await button.click();
      const picker = bed.page.getByRole("dialog", { name: "View" });
      const radios = picker.getByRole("radio");
      await expect(radios).toHaveCount(5);
      const width = bed.page.viewportSize()!.width;
      for (const [i, name] of ["Focus", "Default", "Verbose", "Raw wrapped", "Raw unwrapped"].entries()) {
        await expect(radios.nth(i)).toContainText(name);
        const box = (await radios.nth(i).boundingBox())!;
        expect(box.height, `${name} is a 44px target`).toBeGreaterThanOrEqual(44);
        expect(box.x + box.width, `${name} stays inside the ${info.project.name} screen`).toBeLessThanOrEqual(width);
      }
      // This host sends no transcript: the levels say why, and the raw views remain.
      for (const i of [0, 1, 2]) await expect(radios.nth(i)).toBeDisabled();
      await expect(radios.nth(4), "the view on screen has the focus").toBeFocused();
      await bed.page.keyboard.press("ArrowDown");
      await expect(radios.nth(3), "arrows skip the levels it cannot show").toBeFocused();
      await bed.step("picker", { compare: false });
      await bed.page.keyboard.press("Escape");
      await expect(picker).toHaveCount(0);
      await expect(button, "Escape keeps the view").toHaveText("Raw unwrapped");
      await expect(button, "and focus goes back to the button").toBeFocused();
    });
  });

  test("a seat with no readable transcript opens on its screen and says why in one line", async ({ bed }) => {
    bed.mock.refuseTranscript(imp, "unreadable_format");
    await bed.openSeat(SEATS.imp);
    await expect(bed.page.locator(".fallback")).toHaveText("This seat's transcript is in a format Treehouse does not read yet. Showing the screen.");
    await expect(view(bed)).toHaveCount(0);
    await expect(bed.screen(), "the raw screen is up").toBeVisible();
    await expect(viewButton(bed.page), "the control is still there, naming the screen").toHaveText(/^Raw/);
    await bed.step("no-transcript", { compare: false });
  });

  test("opens on Focus, with the last reply of each turn and a line per tool, and no peer message", async ({ bed }) => {
    await open(bed);
    await expect(viewButton(bed.page)).toHaveText("Focus");
    await expect(view(bed)).toContainText("Why does the wrapped row hide the stripe?");
    await expect(view(bed)).toContainText("The stripe was placed by row number");
    await expect(view(bed)).toContainText("Ran just aterm-client-gate");
    await expect(view(bed), "an earlier reply in the same turn is not Focus").not.toContainText("I will read the component first.");
    await expect(view(bed), "no result text in Focus").not.toContainText("gate line 1");
    await expect(view(bed), "a peer message is not here").not.toContainText("Is the frame fixed");
    await expect(view(bed), "nor the reply that answers it").not.toContainText("Yes, the frame is fixed.");
    await expect(view(bed), "nor the tool step that answers it").not.toContainText("Ran git log");
    await expect(view(bed)).toContainText("Carrying on.");
    await expect(view(bed).locator(".source")).toContainText("From the transcript");
    await bed.step("focus", { compare: false });
  });

  test("Default adds every reply and three lines of a result, Verbose all of it with the raw screen one tap away", async ({ bed }) => {
    await open(bed);
    await pickView(bed.page, "Default");
    await expect(view(bed)).toContainText("I will read the component first.");
    await expect(view(bed)).toContainText("gate line 3");
    await expect(view(bed)).not.toContainText("gate line 4");
    await expect(view(bed)).toContainText("5 more lines. Verbose shows them.");
    await expect(view(bed), "still no peer message").not.toContainText("Is the frame fixed");
    await bed.step("default", { compare: false });

    await pickView(bed.page, "Verbose");
    await expect(view(bed)).toContainText("gate line 8");
    await expect(view(bed)).not.toContainText("more lines");
    await expect(view(bed), "a failed step says so").toContainText("failed");
    await bed.step("verbose", { compare: false });
    await pickView(bed.page, "Raw unwrapped");
    await expect(viewButton(bed.page)).toHaveText("Raw unwrapped");
    await expect(view(bed)).toHaveCount(0);
    await expect(bed.screen()).toBeVisible();
    await pickView(bed.page, "Verbose");
    await expect(view(bed)).toBeVisible();
  });

  test("the stop is the seat's own and kept on the device", async ({ bed }) => {
    await open(bed);
    await pickView(bed.page, "Default");
    await reopenSeat(bed.page, SEATS.imp);
    await expect(bed.page.getByRole("heading", { name: SEATS.imp.name, level: 1 })).toBeAttached();
    await expect(viewButton(bed.page)).toHaveText("Default");
    await bed.switchSeat(SEATS.beetle);
    await expect(viewButton(bed.page), "another seat opens on Focus").toHaveText("Focus");
  });

  test("Raw wrapped and Raw unwrapped take effect at once, and fold the screen as the switch does", async ({ bed }) => {
    await open(bed);
    await pickView(bed.page, "Raw wrapped");
    await expect(viewButton(bed.page)).toHaveText("Raw wrapped");
    await expect(view(bed)).toHaveCount(0);
    await expect(bed.page.locator(".term-host"), "the screen folds its lines").toHaveClass(/wrapping/);
    await pickView(bed.page, "Raw unwrapped");
    await expect(viewButton(bed.page)).toHaveText("Raw unwrapped");
    await expect(bed.page.locator(".term-host"), "and unfolds them").not.toHaveClass(/wrapping/);
  });

  test("a view stored by the old row and the wrap switch comes back as the same choice", async ({ bed }) => {
    // The row kept {level, raw}, and the toolbar switch kept a list of wrapped seats.
    await bed.page.addInitScript((id) => {
      localStorage.setItem("aterm.seat-view.v1", JSON.stringify([[id, { level: "verbose", raw: true }]]));
      localStorage.setItem("aterm.terminal-wrap.v1", JSON.stringify([id]));
    }, imp);
    await open(bed);
    await expect(viewButton(bed.page), "verbose with the screen up, wrapped").toHaveText("Raw wrapped");
    await bed.switchSeat(SEATS.beetle);
    await expect(viewButton(bed.page), "a seat with nothing stored opens on Focus").toHaveText("Focus");
  });

  test("a line the seat writes later arrives on its own, asked for by the cursor", async ({ bed }) => {
    await open(bed);
    await expect(view(bed)).toContainText("Carrying on.");
    bed.mock.appendTranscript(imp, [{ id: "r4", kind: "reply", text: "A line written a moment later.", complete: true }]);
    await expect(view(bed)).toContainText("A line written a moment later.", { timeout: 6000 });
    expect(bed.mock.transcriptCalls.some((call) => call.since !== undefined), "it asked since a cursor").toBe(true);
  });

  test("an item the host calls shared stays and says it may answer another seat", async ({ bed }) => {
    bed.mock.setTranscript(imp, [
      { id: "p1", kind: "prompt", text: "Refactor it." },
      { id: "x1", kind: "peer", text: "ping", from: { role: "eng-platform", identity: "Beetle-Ox" } },
      { id: "r1", kind: "reply", text: "Refactor done.", complete: true, for: "x1", shared: true },
    ]);
    await bed.openSeat(SEATS.imp);
    await expect(view(bed)).toContainText("Refactor done.");
    await expect(view(bed)).toContainText("May answer a message from Beetle-Ox.");
    await expect(view(bed)).not.toContainText("[from eng-platform");
    await bed.step("shared", { compare: false });
    // The marker points at the message, which only the Messages panel shows.
    bed.mock.push({ type: "message", message: { id: "m9", from: "eng-platform Beetle-Ox", target: "frontend-eng", session: imp, state: "delivered", text: "ping" } });
    await view(bed).getByRole("button", { name: "Show the message" }).click();
    await expect(bed.page.getByRole("tab", { name: /^Messages/, selected: true })).toBeVisible();
    const row = bed.page.locator("#message-row-m9");
    await expect(row).toHaveAttribute("aria-expanded", "true");
    await expect(bed.page.getByRole("region", { name: "Full text from Beetle-Ox" })).toContainText("ping");
    await bed.step("shown", { compare: false });
  });

  test("Show earlier reads the page before the first one and keeps the view where it was", async ({ bed }) => {
    const long = Array.from({ length: 120 }, (_, index) => ({ id: `n${index}`, kind: index % 2 === 0 ? "prompt" : "reply", text: `message ${index}` }));
    bed.mock.setTranscript(imp, long);
    await bed.openSeat(SEATS.imp);
    await pickView(bed.page, "Default");
    await expect(view(bed)).toContainText("message 119");
    await expect(view(bed)).not.toContainText("message 69");
    const earlier = view(bed).getByRole("button", { name: "Show earlier" });
    await expect(earlier).toBeVisible();
    await view(bed).locator(".feed").evaluate((feed) => (feed.scrollTop = 0));
    const first = await view(bed).getByText("message 70", { exact: true }).boundingBox();
    await earlier.click();
    await expect(view(bed)).toContainText("message 20");
    expect(bed.mock.transcriptCalls.some((call) => call.before !== undefined), "it asked for the page before").toBe(true);
    const after = await view(bed).getByText("message 70", { exact: true }).boundingBox();
    expect(Math.abs(after!.y - first!.y), "the message the person was reading did not move").toBeLessThan(80);
    await expect(view(bed).getByRole("button", { name: "Show earlier" })).toBeVisible();
  });

  test("a peer message shows in the Messages panel and not in the view", async ({ bed }) => {
    await open(bed);
    bed.mock.push({ type: "message", message: { id: "m1", from: "eng-platform Beetle-Ox", target: "frontend-eng", session: imp, state: "delivered", text: "Is the frame fixed?" } });
    await bed.page.getByRole("tab", { name: /^Messages/ }).click();
    await expect(bed.page.getByText("Is the frame fixed?").first()).toBeVisible();
    await expect(view(bed)).not.toContainText("Is the frame fixed");
  });
});

test.describe("on a desktop", () => {
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "a desktop pane"));

  test("keeps the screen it has always had, named Raw unwrapped, and Focus is one pick away", async ({ bed }) => {
    await open(bed);
    await expect(viewButton(bed.page), "a desktop pane scrolls its long lines").toHaveText("Raw unwrapped");
    await expect(view(bed)).toHaveCount(0);
    await expect(bed.screen()).toBeVisible();
    await pickView(bed.page, "Focus");
    await expect(view(bed)).toContainText("The stripe was placed by row number");
    await bed.step("desktop-focus", { compare: false });
  });
});

// The daemon's own replies for its three fixtures, replayed as it would send them.
const golden = process.env.ATERM_TRANSCRIPT_GOLDEN ?? new URL("../../../aterm/testdata/transcript/", import.meta.url).pathname;

test.describe("the daemon's golden replies", () => {
  test.skip(!existsSync(`${golden}claude.golden.json`), "the daemon's fixtures are not checked out");
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "read on a phone"));

  for (const reader of ["claude", "codex", "opencode"]) {
    test(`${reader}: reads as prompts, replies and tool lines, and shows no peer message`, async ({ bed }) => {
      const reply = JSON.parse(readFileSync(`${golden}${reader}.golden.json`, "utf8")) as { items: { kind: string; text?: string; tool?: { summary: string; ok?: boolean } }[] };
      bed.mock.setTranscript(imp, reply.items);
      await bed.openSeat(SEATS.imp);
      await expect(view(bed)).toContainText(reply.items.find((item) => item.kind === "prompt")!.text!);
      await bed.step(`${reader}-focus`, { compare: false });
      await pickView(bed.page, "Verbose");
      const failed = reply.items.find((item) => item.tool?.ok === false)!;
      await expect(view(bed)).toContainText(failed.tool!.summary);
      await expect(view(bed)).toContainText("failed");
      for (const peer of reply.items.filter((item) => item.kind === "peer")) {
        await expect(view(bed), "a peer message is not drawn").not.toContainText(peer.text!.slice(0, 40));
      }
      await bed.step(`${reader}-verbose`, { compare: false });
    });
  }
});
