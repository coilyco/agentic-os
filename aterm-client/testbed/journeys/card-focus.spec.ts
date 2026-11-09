// COI-3314: typing or dictating into a card's text box must not lose focus or its draft.
import { readFileSync } from "node:fs";
import { card, expect, kindOf, pushOutput, quirk, SEATS, test } from "../harness.ts";
import { captured, type Scene } from "../scenes.ts";

type Bed = Parameters<Parameters<typeof test>[2]>[0]["bed"];

const ask = (id: string, session: string, over: Record<string, unknown> = {}) => ({
  type: "ask",
  ask: { id, session, header: "Scope", question: `Question ${id}?`, options: [{ label: "Yes", description: "" }, { label: "No", description: "" }], allow_other: true, multi: false, ...over },
});
const OTHERS = [SEATS.beetle, SEATS.griffin, SEATS.turtle];

// Three other seats start and stop waiting while the seat prints, around a person typing.
async function churn(bed: Bed, rounds: number): Promise<void> {
  for (let round = 0; round < rounds; round++) {
    OTHERS.forEach((seat, index) => bed.mock.push(ask(`other-${round}-${index}`, seat.id)));
    pushOutput(bed.mock, SEATS.imp.id, `\r\nstreamed line ${round}\r\n`);
    await bed.page.waitForTimeout(120);
    OTHERS.forEach((_, index) => bed.mock.push({ type: "asked", ask_id: `other-${round}-${index}`, state: "cancelled" }));
    await bed.page.waitForTimeout(120);
  }
}

const field = (bed: Bed) => bed.page.locator("#choice-text");

test("a daemon card's text box keeps focus and its draft while other seats toggle", quirk("COI-3314"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask("mine", SEATS.imp.id));
  await card(bed.page).getByRole("button", { name: /Something else/ }).click();
  await expect(field(bed)).toBeFocused();
  await bed.page.keyboard.type("my own words", { delay: 20 });
  await churn(bed, 5);
  await expect(field(bed), "focus left the text box").toBeFocused();
  await expect(field(bed), "the draft was lost").toHaveValue("my own words");
  await bed.page.keyboard.type(" go on", { delay: 20 });
  await expect(field(bed)).toHaveValue("my own words go on");
  await bed.step("still-typing");
});

test("a burst of dictation lands whole while the banner churns", quirk("COI-3314"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask("mine", SEATS.imp.id));
  await card(bed.page).getByRole("button", { name: /Something else/ }).click();
  await expect(field(bed)).toBeFocused();
  const words = Array.from({ length: 14 }, (_, i) => `word${i}`);
  // Wispr Flow-style: whole chunks inserted at once, no key events, other seats toggling.
  await Promise.all([churn(bed, 3), (async () => { for (const word of words) { await bed.page.keyboard.insertText(`${word} `); await bed.page.waitForTimeout(40); } })()]);
  await expect(field(bed)).toHaveValue(words.map((word) => `${word} `).join(""));
  await expect(field(bed)).toBeFocused();
});

test("a screen-read card's text box keeps focus while the seat redraws", quirk("COI-3314"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "desktop", "captures are fixed width");
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, captured("claude-2.1.318-72-01-q1"));
  await card(bed.page).getByRole("button", { name: /Something else/ }).click();
  await expect(field(bed)).toBeFocused();
  await bed.page.keyboard.type("typed in words", { delay: 20 });
  await churn(bed, 4);
  await expect(field(bed), "focus left the text box").toBeFocused();
  await expect(field(bed), "the draft was lost").toHaveValue("typed in words");
});

// The dialog itself ticks, as an away-timer would: its question line changes every beat.
const ticking = (n: number): Scene => {
  const text = readFileSync(new URL("./../fixtures/captures/claude-2.1.318-72-01-q1.txt", import.meta.url), "utf8");
  const lines = text.replace(/\n+$/, "").split("\n").map((line) => (line.includes("Which accent color") ? `${line.trimEnd()} (continuing in ${n}s)` : line));
  return { render: (_cols, rows) => lines.slice(-rows) };
};

test("a screen-read card keeps its text box and draft when the dialog's own text ticks", quirk("COI-3314"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "desktop", "captures are fixed width");
  await bed.openSeat(SEATS.imp);
  let n = 60;
  bed.mock.setScene(SEATS.imp.id, ticking(n));
  await card(bed.page).getByRole("button", { name: /Something else/ }).click();
  await expect(field(bed)).toBeFocused();
  await bed.page.keyboard.type("half a thought", { delay: 20 });
  for (let tick = 0; tick < 8; tick++) {
    bed.mock.setScene(SEATS.imp.id, ticking(--n));
    await bed.page.waitForTimeout(300);
  }
  await expect(field(bed), "focus left the text box").toBeFocused();
  await expect(field(bed), "the draft was lost").toHaveValue("half a thought");
  await bed.page.keyboard.type(" and the rest", { delay: 20 });
  await expect(field(bed)).toHaveValue("half a thought and the rest");
});

test("a card that flickers away and back returns with its text box and draft", quirk("COI-3314"), async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "desktop", "captures are fixed width");
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, captured("claude-2.1.318-72-01-q1"));
  await card(bed.page).getByRole("button", { name: /Something else/ }).click();
  await bed.page.keyboard.type("kept across a redraw", { delay: 20 });
  // The dialog is off screen longer than the client waits before taking the card down.
  bed.mock.setScene(SEATS.imp.id, { render: () => ["", "Redrawing…"] });
  await expect(card(bed.page)).toBeHidden();
  bed.mock.setScene(SEATS.imp.id, captured("claude-2.1.318-72-01-q1"));
  await expect(card(bed.page)).toBeVisible();
  await expect(field(bed), "the text box did not come back").toBeFocused();
  await expect(field(bed), "the draft did not come back").toHaveValue("kept across a redraw");
});

test("a daemon card's text box and draft survive a seat that keeps printing for a while", quirk("COI-3314"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.push(ask("mine", SEATS.imp.id));
  await card(bed.page).getByRole("button", { name: /Something else/ }).click();
  await bed.page.keyboard.type("still here", { delay: 20 });
  // Longer than the client waits before it forgets a screen-read dialog that is gone.
  for (let beat = 0; beat < 14; beat++) {
    pushOutput(bed.mock, SEATS.imp.id, `\r\nstill working ${beat}\r\n`);
    await bed.page.waitForTimeout(500);
  }
  await expect(field(bed), "the text box closed").toBeVisible();
  await expect(field(bed), "the draft was lost").toHaveValue("still here");
  await expect(field(bed)).toBeFocused();
});
