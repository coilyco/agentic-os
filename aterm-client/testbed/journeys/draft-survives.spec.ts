// COI-3533: the words in a seat's message box outlive a closed app, per seat, for a day.
import { expect, SEATS, test, type Bed, seatEntry } from "../harness.ts";

const field = (bed: Bed) => bed.page.getByRole("textbox", { name: /^Message / });
// A reload rests on the seat overview, so the seat is opened again by its tab (COI-3518).
const reopen = async (bed: Bed) => {
  await bed.page.reload();
  await seatEntry(bed.page, SEATS.imp.name).click();
};

test("words typed to a seat are still there after the page is reopened", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await field(bed).fill("Ask Beetle-Ox whether the gate is green");
  await reopen(bed);
  await expect(bed.page.getByRole("heading", { name: SEATS.imp.name, level: 1 })).toBeAttached();
  await expect(field(bed), "the draft is waiting in its seat").toHaveValue("Ask Beetle-Ox whether the gate is green");
  await bed.step("draft-back", { compare: false });
});

test("a draft belongs to its seat, and an emptied box stays empty", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await field(bed).fill("for Imp only");
  await reopen(bed);
  await expect(field(bed)).toHaveValue("for Imp only");
  await field(bed).fill("");
  await reopen(bed);
  await expect(field(bed), "an emptied box is not brought back").toHaveValue("");
  await field(bed).fill("still for Imp");
  await bed.switchSeat(SEATS.beetle);
  await expect(field(bed), "another seat starts empty").toHaveValue("");
  await bed.switchSeat(SEATS.imp);
  await expect(field(bed), "and the first seat's words are waiting").toHaveValue("still for Imp");
});

test("a draft more than a day old is not brought back, since its seat name may be another seat's", async ({ bed }) => {
  const old = Date.now() - 25 * 60 * 60 * 1000;
  await bed.page.addInitScript(([key, at]) => localStorage.setItem("aterm.drafts.v1", JSON.stringify({ [key]: { text: "stale words", at } })), [`local/${SEATS.imp.id}`, old] as const);
  await bed.openSeat(SEATS.imp);
  await expect(field(bed)).toHaveValue("");
});
