// COI-3797 slice 2: one search over every live seat, grouped by seat, and a tap opens
// that seat at the match.
import { expect, loadFonts, SEATS, test } from "../harness.ts";

test.use({ mockOptions: { transcript: true, transcriptSearch: true, transcriptSearchAll: true } });

const filler = (seat: string, from: number, count: number) => Array.from({ length: count }, (_, at) => ({ id: `${seat}-${from + at}`, kind: "reply", text: `${seat} item ${from + at}.`, complete: true }));

test("results from two seats, grouped, and a tap opens the seat at its match", async ({ bed }) => {
  // Beetle said it early in a long transcript, Imp near its end, and Turtle never.
  bed.mock.setTranscript(SEATS.beetle.id, [...filler("beetle", 1, 4), { id: "beetle-early", kind: "prompt", text: "Move the lighthouse backups first." }, ...filler("beetle", 6, 600)]);
  bed.mock.setTranscript(SEATS.imp.id, [...filler("imp", 1, 40), { id: "imp-late", kind: "reply", text: "The lighthouse page is done.", complete: true }, ...filler("imp", 42, 3)]);
  bed.mock.setTranscript(SEATS.turtle.id, filler("turtle", 1, 20));
  await bed.page.addInitScript(() => localStorage.setItem("aterm.seat-view-default.v1", "default"));
  await bed.page.goto("/");
  await loadFonts(bed.page);
  await bed.page.getByRole("tab", { name: /this Mac/ }).click();

  const field = bed.page.getByRole("searchbox", { name: "Search every seat's transcript" });
  await field.fill("lighthouse");
  await expect(bed.page.locator(".seat-search [role=status]")).toHaveText("2 matches in 2 seats");
  const groups = bed.page.locator(".seat-search .seat");
  await expect(groups).toHaveCount(2);
  await expect(groups.filter({ hasText: SEATS.beetle.name })).toContainText("Move the lighthouse backups first.");
  await expect(groups.filter({ hasText: SEATS.imp.name })).toContainText("The lighthouse page is done.");
  await expect(groups.filter({ hasText: SEATS.turtle.name }), "a seat with no match has no group").toHaveCount(0);
  await bed.step("two-seats");

  await groups.filter({ hasText: SEATS.beetle.name }).getByRole("button", { name: /lighthouse backups/ }).click();
  const row = bed.page.locator('.transcript [data-row="beetle-early"]');
  await expect(row, "the seat opens on the match, paged in from item 5 of 606").toBeInViewport();
  await expect(bed.page.getByRole("searchbox", { name: "Search this seat's transcript" })).toHaveValue("lighthouse");
  await expect(bed.page.locator(".transcript .search [role=status]")).toHaveText("1 of 1");
  await bed.step("opened-at-match");
});
