// COI-3482: with the keyboard up on a small phone the card's options stay readable.
import { card, expect, kindOf, quirk, SEATS, test } from "../harness.ts";

const ask = {
  type: "ask",
  ask: {
    id: "short-1",
    session: SEATS.imp.id,
    header: "Accent",
    question: "Which accent color should the client use for the active seat?",
    options: ["Purple", "Teal", "Black", "Gold"].map((label) => ({ label, description: "A description that wraps." })),
    allow_other: true,
    multi: false,
  },
};

// Options whole in the card. At 320x420 the next shows in part and the card scrolls.
for (const [width, height, whole] of [[320, 420, 1], [360, 480, 2], [393, 520, 2]] as const) {
  test(`at ${width}x${height} with the keyboard up, ${whole} option${whole > 1 ? "s" : ""} can be read`, quirk("COI-3482"), async ({ bed }) => {
    test.skip(kindOf(test.info()) !== "phone", "phone");
    await bed.openSeat(SEATS.imp);
    await bed.page.locator("#composer-input").focus();
    await bed.page.setViewportSize({ width, height });
    await expect.poll(() => bed.mock.idleMs(), { timeout: 15000 }).toBeGreaterThan(1500);
    bed.mock.push(ask);
    await expect(card(bed.page)).toBeVisible();
    await bed.step(`keyboard-${width}x${height}`, { compare: false });
    await expect(card(bed.page).getByRole("heading", { name: /accent color/ }), "the question is not on screen").toBeInViewport();
    const rows = card(bed.page).locator("button.option");
    const region = (await card(bed.page).locator(".body").boundingBox())!;
    let readable = 0;
    for (let i = 0; i < (await rows.count()); i++) {
      const box = (await rows.nth(i).boundingBox())!;
      if (box.y >= region.y - 1 && box.y + box.height <= region.y + region.height + 1) readable += 1;
    }
    expect(readable, `${readable} options sit fully inside the card's scrolling area, which is ${Math.round(region.height)}px tall`).toBeGreaterThanOrEqual(whole);
  });
}
