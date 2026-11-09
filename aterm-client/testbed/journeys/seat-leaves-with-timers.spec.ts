// COI-3524: a seat that leaves while its card timers are pending raises no page error.
import { card, expect, SEATS, test } from "../harness.ts";
import { loadSessions } from "../mock-daemon.ts";
import { ACCENT, ClaudeAsk } from "../scenes.ts";

test("a native card goes away, then its seat leaves, and the forget timer fires quietly", async ({ bed }) => {
  const errors: string[] = [];
  bed.page.on("pageerror", (error) => errors.push(error.message));
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, new ClaudeAsk([ACCENT]));
  await expect(card(bed.page)).toBeVisible();
  // The dialog leaves the screen. The card hides, then a 5 second forget timer starts.
  bed.mock.setScene(SEATS.imp.id, { render: () => [] });
  await expect(card(bed.page)).toBeHidden();
  const rest = loadSessions().filter((seat) => seat.name !== SEATS.imp.id);
  bed.mock.push({ type: "sessions", sessions: rest.map(({ scene: _scene, ...view }) => view), terminals: [] });
  await bed.page.waitForTimeout(6500);
  expect(errors, "page errors after the seat left").toEqual([]);
});
