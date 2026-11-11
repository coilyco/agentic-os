// COI-3614: reopening a seat draws what its terminal held at once and asks the host only
// for what came after, by offset. The mock counts its stream, and sends nothing for it.
import { expect, SEATS, test } from "../harness.ts";

test.use({ mockOptions: { offsets: true } });

test("a reopened seat shows the screen it held, though the host sent nothing, and named its offset", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.screen()).toContainText("Churning");
  await bed.switchSeat(SEATS.beetle);
  await expect(bed.screen()).toContainText("The gate is green");
  const before = bed.mock.attachCalls.length;
  await bed.switchSeat(SEATS.imp);
  await expect(bed.screen(), "drawn from what the client held").toContainText("Churning");
  const again = bed.mock.attachCalls.slice(before).filter((call) => call.session === SEATS.imp.id && call.replay);
  expect(again.length, "the seat was attached again").toBeGreaterThan(0);
  expect(again[0]!.offset, "and the host was told where to go on from").toBeGreaterThan(0);
  const first = bed.mock.attachCalls.find((call) => call.session === SEATS.imp.id && call.replay);
  expect(first, "the first open asked for the whole screen").not.toHaveProperty("offset");
  await bed.step("reopened", { compare: false });
});
