// COI-3552: a real harness session, recorded in a pty at this client's own terminal
// size, replayed through the client. Skipped unless RECORDED_FILE names a recording.
import { SEATS, test } from "../harness.ts";
import { loadSessions } from "../mock-daemon.ts";

const file = process.env.RECORDED_FILE;
test.skip(!file, "set RECORDED_FILE to a recording made by record-harness.py");
test.use({ mockOptions: { sessions: loadSessions().map((each) => ({ ...each, scene: "recorded" })) } });

test("a recorded harness session reads on the brown ground", async ({ bed }, info) => {
  await bed.openSeat(SEATS.imp);
  await bed.page.waitForTimeout(2500);
  // The box is what a recording must be made at: record-harness.py takes it as ROWS COLS.
  console.log(`terminal box ${JSON.stringify(bed.mock.boxOf(SEATS.imp.id))} on ${info.project.name}`);
  await bed.page.mouse.move(0, 0);
  await bed.page.screenshot({ path: process.env.SHOT ?? info.outputPath("recorded.png") });
});
