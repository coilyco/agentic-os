import { describe, expect, it } from "vitest";
import { NO_BOX, SHRINK_HOLD_MS, steadyBox } from "./box-rows";

describe("steadyBox", () => {
  it("takes the first box as it is", () => {
    expect(steadyBox(NO_BOX, 5, 0)).toEqual({ asked: 5, since: null });
  });

  it("grows at once, so a taller box is never cut", () => {
    expect(steadyBox({ asked: 5, since: null }, 6, 10)).toEqual({ asked: 6, since: null });
  });

  it("holds a size while a notice comes and goes, however often", () => {
    let box = steadyBox(NO_BOX, 5, 0);
    for (let t = 1000, i = 0; i < 6; i++, t += 2000) {
      box = steadyBox(box, i % 2 === 0 ? 6 : 5, t);
      expect(box.asked, `at ${t}ms`).toBe(6);
    }
  });

  it("lets go of the extra row once the smaller box has lasted", () => {
    const grown = { asked: 6, since: null };
    const first = steadyBox(grown, 5, 1000);
    expect(first).toEqual({ asked: 6, since: 1000 });
    expect(steadyBox(first, 5, 1000 + SHRINK_HOLD_MS - 1).asked).toBe(6);
    expect(steadyBox(first, 5, 1000 + SHRINK_HOLD_MS)).toEqual({ asked: 5, since: null });
  });

  it("starts the count again when the box grows back", () => {
    const first = steadyBox({ asked: 6, since: null }, 5, 1000);
    const back = steadyBox(first, 6, 15000);
    expect(back).toEqual({ asked: 6, since: null });
    expect(steadyBox(back, 5, 16000)).toEqual({ asked: 6, since: 16000 });
  });
});
