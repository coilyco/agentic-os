// COI-3532: Focus, Default and Verbose views of a seat, read from its transcript.
// Peer messages never show in them. The raw screen stays under them and in Verbose.
import { expect, kindOf, SEATS, test, type Bed } from "../harness.ts";

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

const stop = (bed: Bed, name: string) => bed.page.getByRole("radio", { name });
const view = (bed: Bed) => bed.page.locator(".transcript");

async function open(bed: Bed) {
  bed.mock.setTranscript(imp, transcript);
  await bed.openSeat(SEATS.imp);
}

test.describe("on a phone", () => {
  test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "a phone's default view"));

  test("a seat with no readable transcript opens on its screen and says why in one line", async ({ bed }) => {
    bed.mock.refuseTranscript(imp, "unreadable_format");
    await bed.openSeat(SEATS.imp);
    await expect(bed.page.locator(".fallback")).toHaveText("This seat's transcript is in a format Treehouse does not read yet. Showing the screen.");
    await expect(view(bed)).toHaveCount(0);
    await expect(bed.screen(), "the raw screen is up").toBeVisible();
    await expect(bed.page.getByRole("radiogroup", { name: "Seat view" }), "the control is still there").toBeVisible();
    await bed.step("no-transcript", { compare: false });
  });

  test("opens on Focus, with the last reply of each turn and a line per tool, and no peer message", async ({ bed }) => {
    await open(bed);
    await expect(stop(bed, "Focus")).toHaveAttribute("aria-checked", "true");
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
    await stop(bed, "Default").click();
    await expect(view(bed)).toContainText("I will read the component first.");
    await expect(view(bed)).toContainText("gate line 3");
    await expect(view(bed)).not.toContainText("gate line 4");
    await expect(view(bed)).toContainText("5 more lines. Verbose shows them.");
    await expect(view(bed), "still no peer message").not.toContainText("Is the frame fixed");
    await bed.step("default", { compare: false });

    await stop(bed, "Verbose").click();
    await expect(view(bed)).toContainText("gate line 8");
    await expect(view(bed)).not.toContainText("more lines");
    await expect(view(bed), "a failed step says so").toContainText("failed");
    const raw = bed.page.getByRole("button", { name: "Raw screen" });
    await expect(raw).toHaveAttribute("aria-pressed", "false");
    await bed.step("verbose", { compare: false });
    await raw.click();
    await expect(raw).toHaveAttribute("aria-pressed", "true");
    await expect(view(bed)).toHaveCount(0);
    await expect(bed.screen()).toBeVisible();
    await raw.click();
    await expect(view(bed)).toBeVisible();
  });

  test("the stop is the seat's own and kept on the device", async ({ bed }) => {
    await open(bed);
    await stop(bed, "Default").click();
    await bed.page.reload();
    await expect(bed.page.getByRole("heading", { name: SEATS.imp.name, level: 1 })).toBeAttached();
    await expect(stop(bed, "Default")).toHaveAttribute("aria-checked", "true");
    await bed.switchSeat(SEATS.beetle);
    await expect(stop(bed, "Focus"), "another seat opens on Focus").toHaveAttribute("aria-checked", "true");
  });

  test("arrow keys move between the stops, and the stops are 44px targets that fit one row", async ({ bed }, info) => {
    await open(bed);
    await stop(bed, "Focus").focus();
    await bed.page.keyboard.press("ArrowRight");
    await expect(stop(bed, "Default")).toHaveAttribute("aria-checked", "true");
    await expect(stop(bed, "Default")).toBeFocused();
    await bed.page.keyboard.press("ArrowLeft");
    await expect(stop(bed, "Focus")).toHaveAttribute("aria-checked", "true");
    const width = bed.page.viewportSize()!.width;
    for (const name of ["Focus", "Default", "Verbose"]) {
      const box = (await stop(bed, name).boundingBox())!;
      expect(box.height, `${name} is a 44px target`).toBeGreaterThanOrEqual(44);
      expect(box.width, `${name} is a 44px target wide`).toBeGreaterThanOrEqual(44);
      expect(box.x + box.width, `${name} stays inside the ${info.project.name} screen`).toBeLessThanOrEqual(width);
    }
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
      { id: "x1", kind: "peer", text: "[from eng-platform Beetle-Ox] ping", from: { role: "eng-platform", identity: "Beetle-Ox" } },
      { id: "r1", kind: "reply", text: "Refactor done.", complete: true, for: "x1", shared: true },
    ]);
    await bed.openSeat(SEATS.imp);
    await expect(view(bed)).toContainText("Refactor done.");
    await expect(view(bed)).toContainText("May answer another seat's message.");
    await expect(view(bed)).not.toContainText("[from eng-platform");
    await bed.step("shared", { compare: false });
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

  test("keeps the screen it has always had, with Verbose and Raw screen showing, and Focus is one click away", async ({ bed }) => {
    await open(bed);
    await expect(stop(bed, "Verbose")).toHaveAttribute("aria-checked", "true");
    await expect(bed.page.getByRole("button", { name: "Raw screen" })).toHaveAttribute("aria-pressed", "true");
    await expect(view(bed)).toHaveCount(0);
    await expect(bed.screen()).toBeVisible();
    await stop(bed, "Focus").click();
    await expect(view(bed)).toContainText("The stripe was placed by row number");
    await bed.step("desktop-focus", { compare: false });
  });
});
