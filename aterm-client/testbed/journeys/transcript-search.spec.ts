// COI-3797: search a seat's whole transcript, at every size, and jump to a match the
// client had not loaded.
import { expect, kindOf, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { transcript: true, transcriptSearch: true } });

const imp = SEATS.imp.id;
const LENGTH = 1200;
// One phrase near the start, a dozen spread through, and filler everywhere else.
const item = (n: number) => {
  if (n === 3) return { id: `i${n}`, kind: "prompt", text: "Check the lighthouse migration before lunch." };
  if (n % 100 === 50) return { id: `i${n}`, kind: "tool", tool: { name: "Bash", class: "shell", summary: `Ran just gate ${n}`, output: "ok", ok: true } };
  return { id: `i${n}`, kind: "reply", text: `Item number ${n}.`, complete: true };
};
const transcript = Array.from({ length: LENGTH }, (_, at) => item(at + 1));

const field = (bed: Bed) => bed.page.getByRole("searchbox", { name: "Search this seat's transcript" });
const status = (bed: Bed) => bed.page.locator(".transcript .search [role=status]");
const row = (bed: Bed, id: string) => bed.page.locator(`.transcript [data-row="${id}"]`);
const marked = (bed: Bed, name: string) => bed.page.evaluate((key) => (globalThis as unknown as { CSS?: { highlights?: Map<string, Set<unknown>> } }).CSS?.highlights?.get(key)?.size ?? -1, name);

async function openSearch(bed: Bed, desktop: boolean) {
  // Default shows the transcript at every size, where a desktop opens on its screen.
  await bed.page.addInitScript(() => localStorage.setItem("aterm.seat-view-default.v1", "default"));
  bed.mock.setTranscript(imp, transcript);
  await bed.openSeat(SEATS.imp);
  await expect(bed.page.locator(".transcript .feed")).toContainText(`Item number ${LENGTH}.`);
  if (desktop) {
    await bed.page.locator(".transcript .feed").click();
    await bed.page.keyboard.press("Control+f");
  } else {
    await bed.step("search-button");
    await bed.page.getByRole("button", { name: "Search transcript" }).click();
  }
  await expect(field(bed)).toBeFocused();
}

test("no match, one match from early in a long transcript, and many with older and newer", async ({ bed }, info) => {
  const desktop = kindOf(info) === "desktop";
  await openSearch(bed, desktop);

  await field(bed).fill("zebra");
  await expect(status(bed)).toHaveText("No matches");
  await bed.step("no-match");

  // Item 3 of 1,200: the client holds the newest 50, so the host finds it.
  await field(bed).fill("LIGHTHOUSE");
  await expect(status(bed)).toHaveText("1 of 1");
  await expect(row(bed, "i3")).toBeInViewport();
  await expect(row(bed, "i3")).toContainText("lighthouse migration");
  await expect(bed.page.getByRole("button", { name: /Jump to latest/ }), "the newest lines left, so the way back shows").toBeVisible();
  expect(bed.mock.transcriptCalls.some((call) => call.before === "10"), "the match's page was asked for by its cursor").toBe(true);
  const highlights = await marked(bed, "transcript-current");
  if (highlights >= 0) expect(highlights, "the match is marked where the browser can").toBe(1);
  await bed.step("one-match");

  // Twelve tool lines say gate. Newest first, Enter steps older.
  await field(bed).fill("gate");
  await expect(status(bed)).toHaveText("1 of 12");
  await expect(row(bed, "i1150")).toBeInViewport();
  await field(bed).press("Enter");
  await expect(status(bed)).toHaveText("2 of 12");
  await expect(row(bed, "i1050")).toBeInViewport();
  await bed.page.getByRole("button", { name: "Older match" }).click();
  await expect(status(bed)).toHaveText("3 of 12");
  await expect(row(bed, "i950")).toBeInViewport();
  await bed.page.getByRole("button", { name: "Newer match" }).click();
  await expect(status(bed)).toHaveText("2 of 12");
  await expect(row(bed, "i1050")).toBeInViewport();
  await field(bed).press("Shift+Enter");
  await field(bed).press("Shift+Enter");
  await expect(status(bed), "newer than the newest wraps to the oldest").toHaveText("12 of 12");
  await expect(row(bed, "i50")).toBeInViewport();
  await bed.step("many-matches");

  await field(bed).press("Escape");
  await expect(field(bed)).toBeHidden();
  expect(await marked(bed, "transcript-match"), "closing clears the marks").toBeLessThanOrEqual(0);
  await bed.page.getByRole("button", { name: /Jump to latest/ }).click();
  await expect(bed.page.locator(".transcript .feed")).toContainText(`Item number ${LENGTH}.`);
  expect(bed.mock.searchCalls.map((call) => call.query)).toEqual(expect.arrayContaining(["zebra", "LIGHTHOUSE", "gate"]));
});
