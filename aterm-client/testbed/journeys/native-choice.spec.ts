// Journey: Claude Code draws its own AskUserQuestion menu and the client makes a card.
// One question works today. Several do not.
import { card, expect, quirk, SEATS, test } from "../harness.ts";
import { ACCENT, ClaudeAsk, MULTI } from "../scenes.ts";

test("answer one native question", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const dialog = new ClaudeAsk([ACCENT]);
  bed.mock.setScene(SEATS.imp.id, dialog);
  await expect(card(bed.page)).toBeVisible();
  await bed.step("card-shown");
  await card(bed.page).getByRole("button", { name: /Teal/ }).click();
  await expect.poll(() => dialog.phase).toBe("done");
  expect(dialog.answers).toEqual(["Teal"]);
  await expect(card(bed.page)).toBeHidden();
  await bed.step("answered");
});

test("type something on a native question", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const dialog = new ClaudeAsk([ACCENT]);
  bed.mock.setScene(SEATS.imp.id, dialog);
  await card(bed.page).getByRole("button", { name: /Something else/ }).click();
  await expect(bed.page.getByLabel("Your own answer")).toBeVisible();
  await bed.step("text-box-open");
  await bed.page.getByLabel("Your own answer").fill("Cedar");
  await card(bed.page).getByRole("button", { name: "Answer" }).click();
  await expect.poll(() => dialog.phase).toBe("done");
  expect(dialog.answers).toEqual(["Cedar"]);
});

// COI-3243 bugs 2 and 3: three questions, a multi-select with a Next row, then Submit.
// Every screen on the way must stay a card.
test("answer three native questions to the end", quirk("COI-3243 bugs 2 and 3"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const dialog = new ClaudeAsk(MULTI);
  bed.mock.setScene(SEATS.imp.id, dialog);
  await expect(card(bed.page)).toBeVisible();
  await bed.step("question-1");
  await card(bed.page).getByRole("button", { name: /Teal/ }).click();

  await expect(card(bed.page).getByRole("heading", { name: /routes/ })).toBeVisible();
  await bed.step("question-2-multi");
  await card(bed.page).getByRole("button", { name: /Qwen/ }).click();
  await expect(card(bed.page).getByRole("button", { name: /^Next$/ }), "the multi-select shows no Next button").toBeVisible();
  await card(bed.page).getByRole("button", { name: /^Next$/ }).click();

  await expect(card(bed.page).getByRole("heading", { name: /name/ })).toBeVisible();
  await bed.step("question-3");
  await card(bed.page).getByRole("button", { name: /Fern/ }).click();

  await expect(card(bed.page).getByRole("button", { name: /Submit answers/ }), "the review screen is not a card").toBeVisible();
  await bed.step("review");
  await card(bed.page).getByRole("button", { name: /Submit answers/ }).click();
  await expect.poll(() => dialog.phase).toBe("done");
  expect(dialog.answers).toEqual(["Teal", "Qwen", "Fern"]);
});

test("type something in a multi-select", quirk("COI-3243 bug 4"), async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const dialog = new ClaudeAsk(MULTI);
  bed.mock.setScene(SEATS.imp.id, dialog);
  await card(bed.page).getByRole("button", { name: /Purple/ }).click();
  await expect(card(bed.page).getByRole("heading", { name: /routes/ })).toBeVisible();
  await card(bed.page).getByRole("button", { name: /Something else/ }).click();
  await expect(bed.page.getByLabel("Your own answer"), "Type something opens no text box").toBeVisible();
  await bed.step("text-box-open");
});

// Kai, 2026-11-04: the dialog's tab bar is plain text, and she expects real buttons.
test("the question tabs are buttons", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, new ClaudeAsk(MULTI));
  await expect(card(bed.page)).toBeVisible();
  for (const header of ["Accent", "Routes", "Name", "Submit"]) {
    await expect(card(bed.page).getByRole("button", { name: new RegExp(header) }), `${header} is text, not a button`).toBeVisible();
  }
  await expect(card(bed.page).getByText("Question 1 of 3"), "the card does not say which question it is").toBeVisible();
  await bed.step("tabs");
});

test("a tab goes to that question and back, and Submit goes to the review", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  const dialog = new ClaudeAsk(MULTI);
  bed.mock.setScene(SEATS.imp.id, dialog);
  const tab = (name: RegExp) => card(bed.page).getByRole("group", { name: "Questions" }).getByRole("button", { name });
  await tab(/Routes/).click();
  await expect(card(bed.page).getByRole("heading", { name: /routes/ })).toBeVisible();
  await expect(card(bed.page).getByText("Question 2 of 3")).toBeVisible();
  await expect(tab(/Routes/)).toHaveAttribute("aria-current", "step");
  await tab(/Accent/).click();
  await expect(card(bed.page).getByText("Question 1 of 3")).toBeVisible();
  await tab(/Submit/).click();
  await expect(card(bed.page).getByText("Review", { exact: true })).toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: /Submit answers/ })).toBeVisible();
  await bed.step("review-from-a-tab");
  await tab(/Name/).click();
  await expect(card(bed.page).getByText("Question 3 of 3")).toBeVisible();
});

test("an answered question is marked on its tab", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, new ClaudeAsk(MULTI));
  await card(bed.page).getByRole("button", { name: /Teal/ }).click();
  await expect(card(bed.page).getByText("Question 2 of 3")).toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: "Accent, answered" })).toBeVisible();
  await expect(card(bed.page).getByRole("button", { name: "Routes", exact: true })).toBeVisible();
});
