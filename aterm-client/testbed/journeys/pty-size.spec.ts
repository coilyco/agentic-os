// Found by the bed: blank rows under a harness box count as part of the box. A short
// conversation in a tall pane then asks the PTY for 5 more rows on every resize, forever.
import { expect, kindOf, SEATS, test } from "../harness.ts";
import { sceneFor } from "../scenes.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "a tall pane"));

test("a short conversation does not grow the PTY forever", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, sceneFor("claude-fresh", {} as never));
  await expect(bed.screen()).toContainText("Hello. What should I work on?");
  await bed.page.waitForTimeout(2500);
  const rows = bed.mock.boxOf(SEATS.imp.id)!.rows;
  await bed.page.waitForTimeout(2500);
  expect(bed.mock.boxOf(SEATS.imp.id)!.rows, `rows asked for, ${rows} 2.5s earlier`).toBe(rows);
  await bed.step("fresh", { compare: false });
});
