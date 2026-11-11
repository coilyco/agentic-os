// COI-3647: a view switch lands on the last item, and the view follows arrivals.
import { expect, SEATS, test, type Bed } from "../harness.ts";

test.use({ mockOptions: { transcript: true } });

const imp = SEATS.imp.id;
const turn = (n: number) => [
  { id: `p${n}`, kind: "prompt", text: `Question number ${n}, with enough words to take a line or two on a phone.` },
  { id: `t${n}`, kind: "tool", tool: { name: "Bash", class: "shell", summary: `Ran step ${n}`, output: "a\nb\nc\nd\ne\nf", ok: true } },
  { id: `r${n}`, kind: "reply", text: `Answer number ${n}. It says a few things so the row has some height.`, complete: true },
];
// Far more than one screen at every size in the bed, in every view.
const history = Array.from({ length: 40 }, (_, i) => turn(i + 1)).flat();

const stop = (bed: Bed, name: string) => bed.page.getByRole("radio", { name });
const feed = (bed: Bed) => bed.page.locator(".transcript .feed");
const gap = (bed: Bed) => feed(bed).evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight);
const atEnd = async (bed: Bed, why: string) => {
  await expect(feed(bed)).toBeVisible();
  await expect.poll(() => gap(bed), { message: why, timeout: 4000 }).toBeLessThan(4);
};

test("every switch between Focus, Default and Verbose lands on the last item and keeps following", async ({ bed }) => {
  bed.mock.setTranscript(imp, history);
  await bed.openSeat(SEATS.imp);
  await stop(bed, "Focus").click();
  await atEnd(bed, "Focus opens at the end");
  for (const name of ["Default", "Verbose", "Focus", "Verbose", "Default"]) {
    await stop(bed, name).click();
    await expect(stop(bed, name)).toHaveAttribute("aria-checked", "true");
    await atEnd(bed, `${name} lands at the end`);
  }
  await expect(bed.page.getByRole("button", { name: /Jump to latest/ })).toHaveCount(0);
  bed.mock.appendTranscript(imp, turn(41));
  await expect(feed(bed)).toContainText("Answer number 41.", { timeout: 6000 });
  await atEnd(bed, "a new arrival is followed after the switches");
});

test("coming back from the raw screen lands on the last item and keeps following", async ({ bed }) => {
  bed.mock.setTranscript(imp, history);
  await bed.openSeat(SEATS.imp);
  await stop(bed, "Focus").click();
  await stop(bed, "Verbose").click();
  await atEnd(bed, "Verbose opens at the end");
  const raw = bed.page.getByRole("button", { name: "Raw screen" });
  await raw.click();
  await expect(feed(bed)).toHaveCount(0);
  // The seat wrote more while its screen was up.
  bed.mock.appendTranscript(imp, [turn(41), turn(42), turn(43)].flat());
  await raw.click();
  await expect(feed(bed)).toContainText("Answer number 43.", { timeout: 6000 });
  await atEnd(bed, "the transcript is back at the end");
  bed.mock.appendTranscript(imp, turn(44));
  await expect(feed(bed)).toContainText("Answer number 44.", { timeout: 6000 });
  await atEnd(bed, "and follows");
});
