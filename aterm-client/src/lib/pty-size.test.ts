import { describe, expect, it } from "vitest";
import { boxToReport } from "./pty-size";

const own = { rows: 50, cols: 200 };

describe("boxToReport", () => {
  it("reports what the host measured", () => {
    expect(boxToReport({ rows: 28, cols: 47 }, true, own)).toEqual({ rows: 28, cols: 47 });
    expect(boxToReport({ rows: 28, cols: 47 }, false, own)).toEqual({ rows: 28, cols: 47 });
  });

  it("sends nothing when a panning host cannot be measured, so the PTY's size is never claimed as a box", () => {
    expect(boxToReport(undefined, true, own)).toBeNull();
  });

  it("keeps xterm's own size for a daemon that does not size the PTY", () => {
    expect(boxToReport(undefined, false, own)).toEqual(own);
  });
});
