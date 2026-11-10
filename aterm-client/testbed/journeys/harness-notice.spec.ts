// COI-3576: on a busy Claude seat a notice came and went. The client asked the PTY for a
// new size each time, and Claude's own input box popped into view above the composer.
import { expect, kindOf, SEATS, test } from "../harness.ts";
import { claudeSession, type Scene } from "../scenes.ts";

test.beforeEach(({}, info) => test.skip(kindOf(info) !== "desktop", "a tall pane"));

const NOTICE = "* No checker is installed, or the one you named in `checker` is missing";
const DASHED = "──⏵⏵─bypass─permissions─on─(shift+tab─to─cycle)─· ← 1 agent";

type Mode = "plain" | "notice" | "glitch";

/** A busy seat with the plain box, a notice under the rule, or the frame Kai saw. */
function busy(): Scene & { mode: Mode } {
  const scene = {
    mode: "plain" as Mode,
    render(cols: number, rows: number) {
      const under = scene.mode === "notice" ? [NOTICE, ""] : [""];
      const lines = claudeSession({ working: true, underRule: under }).render(cols, rows);
      // No bottom rule, the notice, and the footer drawn over a rule.
      if (scene.mode === "glitch") lines.splice(lines.length - 3, 3, "", NOTICE, DASHED);
      return lines;
    },
  };
  return scene;
}

const clip = (page: import("@playwright/test").Page) =>
  page.evaluate(() => (document.querySelector(".xterm") as HTMLElement | null)?.style.clipPath ?? "");

test("a notice that comes and goes does not move the PTY size each time", async ({ bed }) => {
  const scene = busy();
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, scene);
  await expect.poll(() => clip(bed.page), { message: "Claude's box is not hidden" }).not.toBe("");
  const plain = bed.mock.boxOf(SEATS.imp.id)!.rows;
  scene.mode = "notice";
  bed.mock.setScene(SEATS.imp.id, scene);
  await expect.poll(() => bed.mock.boxOf(SEATS.imp.id)!.rows, { message: `rows asked with a plain box: ${plain}` }).toBeGreaterThan(plain);
  const grown = bed.mock.boxOf(SEATS.imp.id)!.rows;
  for (let flap = 1; flap <= 3; flap++) {
    for (const mode of ["plain", "notice"] as const) {
      scene.mode = mode;
      bed.mock.setScene(SEATS.imp.id, scene);
      await bed.page.waitForTimeout(1500);
      expect(bed.mock.boxOf(SEATS.imp.id)!.rows, `rows asked in flap ${flap} with the ${mode} box`).toBe(grown);
    }
  }
  await bed.step("notice", { compare: false });
});

test("a frame that is not the box does not put Claude's input in view, and the box still goes in time", async ({ bed }) => {
  const scene = busy();
  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, scene);
  await expect.poll(() => clip(bed.page), { message: "Claude's box is not hidden" }).not.toBe("");
  scene.mode = "glitch";
  bed.mock.setScene(SEATS.imp.id, scene);
  await bed.page.waitForTimeout(1500);
  expect(await clip(bed.page), "the box was let go of in 1.5s").not.toBe("");
  await bed.step("glitch", { compare: false });
  await expect.poll(() => clip(bed.page), { timeout: 8000, message: "the crop never lets go of a screen with no box" }).toBe("");
});
