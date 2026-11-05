// COI-3324: the phone's bottom strip is 44px targets, opening on a tab with content.
import { expect, kindOf, SEATS, test } from "../harness.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "the bottom strip is the phone layout"));

test("every tab in the strip is a 44px target, and the strip keeps its height", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const tabs = bed.page.getByRole("tab", { name: /^(Messages|Views|Browser|Terminal)/ });
  await expect(tabs).toHaveCount(4);
  for (const tab of await tabs.all()) {
    const box = await tab.boundingBox();
    expect(box?.height, `${await tab.textContent()} is shorter than a finger`).toBeGreaterThanOrEqual(44);
    expect(box?.width, `${await tab.textContent()} is narrower than a finger`).toBeGreaterThanOrEqual(44);
  }
  await bed.step("strip");
});

test("a frontend seat with no browser page opens on Messages, not the empty Browser", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.page.getByRole("tab", { name: /^Messages/ })).toHaveAttribute("aria-selected", "true");
  await expect(bed.page.getByRole("tab", { name: /^Browser/ })).toHaveAttribute("aria-selected", "false");
  await expect(bed.page.getByText(/doesn't stream a browser yet/), "the empty Browser text is the first thing on the strip").toBeHidden();
});
