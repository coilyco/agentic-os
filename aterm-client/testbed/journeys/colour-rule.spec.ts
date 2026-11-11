// COI-3583: one busy seat with every colour a reader meets, for the colour rule's before
// and after. It saves a picture and compares none, since the page shows it by eye.
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
  await bed.step("busy-seat", { compare: false });
});
