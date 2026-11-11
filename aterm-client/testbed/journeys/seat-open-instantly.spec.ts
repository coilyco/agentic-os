// COI-3534: a seat opens on the transcript this device last read, then catches up, with a
// marker where new output starts. The host's answer is held so the first paint is seen.
import { expect, kindOf, reopenSeat, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { transcript: true } });
// A desktop opens on the raw screen. A phone opens on the transcript.
test.beforeEach(({}, info) => test.skip(kindOf(info) !== "phone", "a phone's default view"));

const imp = SEATS.imp.id;
const said = (n: number) => ({ id: `r${n}`, kind: "reply", text: `Reply number ${n}.`, complete: true });
const seen = [{ id: "p1", kind: "prompt", text: "Why is the stripe wrong?" }, said(1), said(2)];
const feed = (bed: Bed) => bed.page.locator(".transcript");
const source = (bed: Bed) => feed(bed).getByRole("status").last();

// The write to IndexedDB is async, so a journey waits for it before it reloads. Opening a
// database that is not there would create it empty, so the probe looks first.
const kept = (bed: Bed) =>
  bed.page.evaluate(async () => {
    const found = await indexedDB.databases();
    if (!found.some((db) => db.name === "aterm.transcripts.v1")) return 0;
    return new Promise<number>((resolve) => {
      const open = indexedDB.open("aterm.transcripts.v1");
      open.onerror = () => resolve(0);
      open.onsuccess = () => {
        const db = open.result;
        const count = db.transaction("pages").objectStore("pages").count();
        count.onsuccess = () => (db.close(), resolve(count.result));
        count.onerror = () => (db.close(), resolve(0));
      };
    });
  });

async function readOnce(bed: Bed) {
  bed.mock.setTranscript(imp, seen);
  await bed.openSeat(SEATS.imp);
  await expect(feed(bed)).toContainText("Reply number 2.");
  await expect.poll(() => kept(bed), { timeout: 8000 }).toBe(1);
}

test("a reopened seat paints what this device read at once, says it is catching up, then marks what is new", async ({ bed }) => {
  await readOnce(bed);
  // Focus shows a turn's last reply, so a new question is what the marker lands on.
  bed.mock.appendTranscript(imp, [{ id: "p2", kind: "prompt", text: "And the label?" }, said(3), said(4)]);
  bed.mock.holdTranscript(imp, 2500);
  await reopenSeat(bed.page, SEATS.imp);
  await expect(feed(bed), "the old page paints before the host answers").toContainText("Reply number 2.");
  await expect(feed(bed)).not.toContainText("Reply number 3.");
  await expect(source(bed)).toContainText("Showing what this device read");
  await expect(source(bed)).toContainText("Catching up.");
  await bed.step("catching-up", { compare: false });
  await expect(feed(bed)).toContainText("Reply number 4.", { timeout: 6000 });
  const marker = feed(bed).getByRole("separator");
  await expect(marker).toHaveText("New since you were last here");
  await expect(marker, "the marker sits above the first new line").toBeInViewport();
  const above = await marker.evaluate((el) => el.nextElementSibling?.textContent ?? "");
  expect(above).toContain("And the label?");
  await expect(source(bed)).toContainText("From the transcript");
  await bed.step("marked", { compare: false });
});

test("nothing new means no marker, and the marker clears once the newest line has been in view a while", async ({ bed }) => {
  await readOnce(bed);
  await reopenSeat(bed.page, SEATS.imp);
  await expect(feed(bed)).toContainText("Reply number 2.");
  await expect(source(bed)).toContainText("From the transcript");
  await expect(feed(bed).getByRole("separator")).toHaveCount(0);
  bed.mock.appendTranscript(imp, [said(3)]);
  await reopenSeat(bed.page, SEATS.imp);
  await expect(feed(bed).getByRole("separator")).toHaveCount(1);
  await expect(feed(bed).getByRole("separator"), "the person has seen it, so it goes").toHaveCount(0, { timeout: 15000 });
});

test("a host that does not answer leaves the kept page up and says so", async ({ bed }) => {
  await readOnce(bed);
  bed.mock.refuseTranscript(imp, null);
  bed.mock.holdTranscript(imp, 60_000);
  await reopenSeat(bed.page, SEATS.imp);
  await expect(feed(bed)).toContainText("Reply number 2.");
  await expect(source(bed)).toContainText("Catching up.");
});

test("a seat that moved on past the kept page drops it and marks everything shown as new", async ({ bed }) => {
  await readOnce(bed);
  bed.mock.appendTranscript(imp, Array.from({ length: 60 }, (_, at) => said(100 + at)));
  await reopenSeat(bed.page, SEATS.imp);
  await expect(feed(bed)).toContainText("Reply number 159.");
  await expect(feed(bed)).not.toContainText("Why is the stripe wrong?");
  await expect(feed(bed).getByRole("separator")).toHaveText("New since you were last here");
});

test("a seat that refuses the read still falls back to its screen with a page kept", async ({ bed }) => {
  await readOnce(bed);
  bed.mock.refuseTranscript(imp, "unreadable_format");
  await reopenSeat(bed.page, SEATS.imp);
  await expect(bed.page.getByText("Showing the screen.")).toBeVisible();
});
