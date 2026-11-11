// COI-3306: a seat stages a command on the shell beside it. The shell comes forward with
// the note over it, and the line goes when it is run. The daemon types the line itself.
import type { Scene } from "../scenes.ts";
import { expect, SEATS, test } from "../harness.ts";

const BODY = "git push origin main";
const NOTE = "Push once the gate is green";

// A shell with the staged command already on its input line, as the daemon leaves it.
const shell = (line: string): Scene => {
  let text = line;
  return {
    render: () => [`$ ${text}`],
    input: (data) => {
      text = /[\r\n]/.test(data) ? "" : text + data;
      return true;
    },
  };
};

const note = (bed: { page: import("@playwright/test").Page }, who: string) => bed.page.getByRole("region", { name: `A command ${who} staged` });

test("a command staged for the seat on screen brings its shell forward, with the note, and Enter runs it", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  const terminal = bed.mock.addTerminal(SEATS.griffin.id, shell(BODY));
  bed.mock.push({ type: "staged", session: terminal, label: SEATS.griffin.id, body: BODY, text: NOTE });
  await expect(bed.page.locator("#side-tab-terminal"), "the Terminal tab is the one showing").toHaveAttribute("aria-selected", "true");
  await expect(note(bed, "Griffin-Goose")).toContainText(BODY);
  await expect(note(bed, "Griffin-Goose")).toContainText(NOTE);
  await expect(bed.page.locator(".pane .xterm-rows"), "the line is on the shell's input").toContainText(`$ ${BODY}`);
  await expect(bed.page.locator(".pane .xterm-helper-textarea"), "the cursor is in the shell, so Enter is one key").toBeFocused();
  await bed.step("staged", { compare: false });
  await bed.page.keyboard.press("Enter");
  await expect.poll(() => bed.mock.typed(terminal)).toContain("\r");
  await expect(note(bed, "Griffin-Goose"), "the note goes with the line").toBeHidden();
});

test("a command staged for another seat waits in its shell until that seat is opened", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  const terminal = bed.mock.addTerminal(SEATS.beetle.id, shell(BODY));
  bed.mock.push({ type: "staged", session: terminal, label: SEATS.beetle.id, body: BODY, text: "" });
  await expect(bed.page.locator('#side-tab-terminal[aria-selected="true"]'), "the seat on screen keeps its own tab").toHaveCount(0);
  await bed.switchSeat(SEATS.beetle);
  // The panel may be put away, as it is on a phone and on a wide screen until opened.
  const tab = bed.page.locator("#side-tab-terminal");
  if (!(await tab.isVisible())) await bed.page.getByRole("button", { name: /Side panel|Show panel/ }).first().click();
  await tab.click();
  await expect(note(bed, "Beetle-Ox")).toContainText(BODY);
  await expect(note(bed, "Beetle-Ox").locator(".why"), "no note, no empty line").toHaveCount(0);
});

test("Ctrl+C clears the note too", async ({ bed }) => {
  await bed.openSeat(SEATS.griffin);
  const terminal = bed.mock.addTerminal(SEATS.griffin.id, shell(BODY));
  bed.mock.push({ type: "staged", session: terminal, label: SEATS.griffin.id, body: BODY, text: NOTE });
  await expect(note(bed, "Griffin-Goose")).toBeVisible();
  await bed.page.keyboard.press("Control+C");
  await expect(note(bed, "Griffin-Goose")).toBeHidden();
});
