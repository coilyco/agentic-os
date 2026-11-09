// COI-3447: a dialog taller than a phone's box is drawn whole once rows are asked for.
// The mock draws a capture's last `rows` rows, as Claude does. See the sizing reference.
import { card, expect, kindOf, SEATS, test } from "../harness.ts";
import { captured, sceneFor } from "../scenes.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "a desktop box already holds the dialog"));

// Real Claude Code 2.1.319, a single-select of four long options, whole at 34 by 60.
const WHOLE = "claude-2.1.319-34x60-single-whole";

test("a dialog taller than the box is asked for rows until it is drawn whole, and the rows go when it is gone", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  // A phone with its keyboard open shows about 16 rows, where a tall dialog is lost.
  await bed.page.setViewportSize({ width: bed.page.viewportSize()!.width, height: 420 });
  // The rows asked for with an ordinary screen, whose input box is cropped and counted.
  bed.mock.setScene(SEATS.imp.id, sceneFor("claude-fresh", {} as never));
  await expect(bed.screen()).toContainText("Hello. What should I work on?");
  await bed.page.waitForTimeout(1000);
  const resting = bed.mock.boxOf(SEATS.imp.id)!.rows;
  bed.mock.setScene(SEATS.imp.id, captured(WHOLE));
  await expect(card(bed.page).getByRole("button", { name: /Start a new project/ }), "the first option is off the screen and was never asked for").toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: /Write or review docs/ })).toBeVisible();
  await expect(card(bed.page).getByText(/off the top/), "the card still says options are lost").toHaveCount(0);
  const asked = bed.mock.boxOf(SEATS.imp.id)!.rows;
  expect(asked, "the PTY was not asked for more rows than the box shows").toBeGreaterThan(resting);
  expect(asked, "more rows than the cap").toBeLessThanOrEqual(resting + 96);
  await bed.step("whole", { compare: false });
  bed.mock.setScene(SEATS.imp.id, sceneFor("claude-fresh", {} as never));
  await expect.poll(() => bed.mock.boxOf(SEATS.imp.id)!.rows, { message: "the extra rows stayed after the dialog went", timeout: 10_000 }).toBe(resting);
});

test("a dialog that already fits does not grow the PTY", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const resting = bed.mock.boxOf(SEATS.imp.id)!.rows;
  bed.mock.setScene(SEATS.imp.id, sceneFor("claude-ask-single", {} as never));
  await expect(card(bed.page)).toBeVisible();
  await bed.page.waitForTimeout(2000);
  expect(bed.mock.boxOf(SEATS.imp.id)!.rows).toBe(resting);
});
