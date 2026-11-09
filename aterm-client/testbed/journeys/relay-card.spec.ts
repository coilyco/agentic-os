// COI-3241: a worker's question relayed to the director's pane. Both rows stay marked.
import { card, expect, kindOf, SEATS, test } from "../harness.ts";

const relayed = (over: Record<string, unknown> = {}) => ({
  type: "ask",
  ask: {
    id: "relay-1",
    session: SEATS.beetle.id,
    from: "eng-platform Beetle-Ox",
    header: "Scope",
    question: "Ship the migration tonight, or wait for review?",
    options: [
      { label: "Tonight", description: "Faster, no second look." },
      { label: "Wait", description: "Safer, a day later." },
    ],
    allow_other: false,
    multi: false,
    relay: true,
    card_seat: SEATS.griffin.id,
    ...over,
  },
});

test("a worker's question shows on the director's pane, naming the worker", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  bed.mock.push(relayed());
  await expect(card(bed.page)).toBeVisible();
  await expect(card(bed.page)).toContainText("Beetle-Ox is asking");
  await bed.step("relayed-card");
  await card(bed.page).getByRole("button", { name: /Wait/ }).click();
  await expect.poll(() => bed.mock.answers).toEqual([{ ask_id: "relay-1", picks: [1] }]);
  await expect(card(bed.page)).toBeHidden();
});

test("the worker's own pane says where its question is, and goes there", async ({ bed }) => {
  await bed.openSeat(SEATS.beetle);
  bed.mock.push(relayed());
  const notice = bed.page.getByRole("status").filter({ hasText: /question is on Griffin-Goose/ });
  await expect(notice).toBeVisible();
  await expect(card(bed.page), "the card shows on the worker's pane too").toBeHidden();
  // The text size button shows at 320px and is not what this step is about.
  await bed.step("on-the-askers-pane", { hide: [".size"] });
  await bed.page.getByRole("button", { name: /Open Griffin-Goose/ }).click();
  await expect(card(bed.page)).toBeVisible();
  await expect(card(bed.page)).toContainText("Beetle-Ox is asking");
});

test("with no director up the card shows on the worker, as an ask_choice does", async ({ bed }) => {
  await bed.openSeat(SEATS.beetle);
  bed.mock.push(relayed({ card_seat: "" }));
  await expect(card(bed.page)).toBeVisible();
  await expect(bed.page.getByRole("status").filter({ hasText: /question is on/ })).toHaveCount(0);
});

test("a repeated ask with a known id moves the card", async ({ bed }) => {
  await bed.openSeat(SEATS.beetle);
  bed.mock.push(relayed({ card_seat: "" }));
  await expect(card(bed.page)).toBeVisible();
  bed.mock.push(relayed());
  await expect(card(bed.page), "the daemon moved it to the director").toBeHidden();
  await expect(bed.page.getByRole("status").filter({ hasText: /question is on Griffin-Goose/ })).toBeVisible();
});

test("both rows are marked, the worker's saying where", async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "desktop", "a phone keeps its rows in a sheet");
  await bed.openSeat(SEATS.turtle);
  bed.mock.push(relayed());
  // By id: a row that names another seat in its state also matches that seat's name.
  const rowOf = (seat: { id: string }) => bed.page.locator(`#tab-session-${seat.id}`);
  await expect(rowOf(SEATS.beetle)).toContainText("asking you, on Griffin-Goose");
  await expect(rowOf(SEATS.griffin)).toContainText("Beetle-Ox asks you");
  await bed.step("both-rows");
});
