// COI-3583: one busy seat with every colour a reader meets, for the colour rule's before
// and after. It saves a picture and compares none, since the page shows it by eye.
import { writeFileSync } from "node:fs";
import { expect, SEATS, test } from "../harness.ts";

const esc = (code: string, text: string) => `\x1b[${code}m${text}\x1b[0m`;
const ramp = (from: number) => [0, 1, 2, 3, 4, 5, 6, 7].map((n) => esc(`${from + n}`, "███")).join(" ");
const screen = [
  `${esc("1;36", "●")} ${esc("1", "Update(aterm-client/src/lib/choices.ts)")}`,
  `  ${esc("90", "└")} Updated ${esc("34", "aterm-client/src/lib/choices.ts")} with 6 additions and 2 removals`,
  `    ${esc("90", "214")}  ${esc("31", "-  const next = rows.find(isNext);")}`,
  `    ${esc("90", "215")}  ${esc("32", "+  const next = rows.findLast(isNext) ?? rows.find(isNext);")}`,
  `    ${esc("90", "216")}  ${esc("32", "+  // a wrapped label is one row")}`,
  "",
  `${esc("1;36", "●")} ${esc("1", "Bash(just aterm-client-gate)")}`,
  `  ${esc("90", "└")} ${esc("32", "✓")} svelte-check found 0 errors and 0 warnings`,
  `    ${esc("32", "✓")} 412 tests passed`,
  `    ${esc("33", "⚠")} chunk is larger than 500 kB after minification`,
  `    ${esc("31", "✗")} testbed/journeys/draft-images.spec.ts ${esc("90", "(2 failed)")}`,
  "",
  `${esc("35", "◆")} ${esc("36", "https://github.com/coilyco/agentic-os/pull/346")}   ${esc("2", "dim note")}  ${esc("1", "bold")}  ${esc("4", "underline")}`,
  "",
  `  normal  ${ramp(30)}`,
  `  bright  ${ramp(90)}`,
].join("\r\n");

test("a busy seat shows its whole palette", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, { render: () => screen.split("\r\n") });
  await expect(bed.screen()).toContainText("bright");
  // Three other seats wait on a person, so the list shows a cue for each.
  const waiting = [SEATS.griffin, SEATS.beetle, SEATS.turtle];
  waiting.forEach((seat, n) => bed.mock.push({ type: "ask", ask: { id: `ask-${n}`, session: seat.id, header: "Scope", question: "Keep the alerts pinned?", options: [{ label: "Pinned", description: "Always visible." }, { label: "Behind a button", description: "One tap away." }], allow_other: false, multi: false } }));
  // A phone keeps the list behind a button, so only a wide screen shows the cues.
  if ((bed.page.viewportSize()?.width ?? 0) > 900) await expect(bed.page.locator('.seat[data-glow="true"]')).toHaveCount(waiting.length);
  await bed.step("busy-seat", { compare: false });
  // Boxes the hue count leaves out: the terminal's own colours and the creature art.
  const skip = await bed.page.evaluate(() => [...document.querySelectorAll(".xterm, img.creature, img.mark")].map((node) => {
    const { x, y, width, height } = node.getBoundingClientRect();
    return [x, y, width, height].map(Math.round);
  }));
  writeFileSync(test.info().outputPath("skip-boxes.json"), JSON.stringify(skip));
});
