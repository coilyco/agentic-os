// COI-3340: the line a seat writes about what it is doing. Cut to the row,
// flagged once old, and absent when the seat sent none.
import { expect, kindOf, SEATS, test } from "../harness.ts";
import type { Locator } from "@playwright/test";

const LONG = "Checking whether a card's text box keeps focus on a phone"; // 57, the daemon's cap
const OLD = "Waiting for the daemon to come back after its restart";
const seat = (name: string, identity: string, extra: object) => ({
  name,
  role: "eng-platform",
  identity,
  seat: "claude",
  ready: true,
  bracketed_paste: true,
  kai_drafting: false,
  pending: 0,
  scene: "claude-idle",
  ...extra,
});
const NARROW = ["phone-320", "desktop-1440", "desktop-1920"];

const fits = (line: Locator) =>
  line.evaluate((el) => ({ cut: el.scrollWidth > el.clientWidth, inside: el.getBoundingClientRect().right <= el.closest("[role=tab]")!.getBoundingClientRect().right }));

test("a seat's line fits its row, an old one says how old, and none leaves the row alone", async ({ bed }, info) => {
  bed.mock.addSession(seat("eng-platform-fresh-ox", "Fresh-Ox", { activity: LONG, activity_age_seconds: 40 }));
  bed.mock.addSession(seat("eng-platform-stale-ox", "Stale-Ox", { activity: OLD, activity_age_seconds: 3 * 3600 + 120 }));
  await bed.openSeat(SEATS.imp);
  if (kindOf(info) !== "desktop") await bed.page.getByRole("button", { name: /Seats, starting a seat, hosts and settings/ }).click();

  const fresh = bed.page.getByRole("tab", { name: /Fresh-Ox.*says Checking whether/ }).first();
  const line = fresh.locator(".activity .line");
  await expect(line).toHaveText(LONG);
  await expect(fresh.locator(".age"), "a recent line carries no age").toHaveCount(0);
  const shape = await fits(line);
  expect(shape.inside, "the line stays inside its row").toBe(true);
  if (NARROW.includes(info.project.name)) expect(shape.cut, "57 characters are cut with an ellipsis at this width").toBe(true);

  const stale = bed.page.getByRole("tab", { name: /Stale-Ox.*set 3h ago/ }).first();
  await expect(stale.locator(".activity")).toHaveAttribute("data-stale", "true");
  await expect(stale.locator(".age"), "an old line says how old").toHaveText("3h ago");
  expect((await fits(stale.locator(".activity .line"))).inside).toBe(true);

  const none = bed.page.getByRole("tab", { name: new RegExp(SEATS.beetle.name) }).first();
  await expect(none.locator(".activity"), "no field, no box").toHaveCount(0);
  await bed.step("rows");
});

test("a phone's seat chip carries the line and grows no further than 190px for it", async ({ bed }, info) => {
  test.skip(kindOf(info) !== "phone", "chips are a phone's");
  bed.mock.addSession(seat("eng-platform-fresh-ox", "Fresh-Ox", { activity: LONG, activity_age_seconds: 40 }));
  await bed.page.goto("/");
  await bed.page.getByRole("tab", { name: /this Mac/ }).click();
  const chip = bed.page.locator(".seats").getByRole("tab", { name: /Fresh-Ox/ });
  await expect(chip.locator(".activity .line")).toHaveText(LONG);
  const box = await chip.locator(".activity").boundingBox();
  expect(box?.width, "the chip grew past its 139 to 163px, to carry the line").toBeGreaterThan(170);
  expect(box?.width, "and no further than 190px").toBeLessThanOrEqual(190);
  await bed.step("chips");
});
