import { afterEach, describe, expect, it, vi } from "vitest";
import { safely, watchDelay } from "./pane-watch";

afterEach(() => vi.restoreAllMocks());

describe("watchDelay", () => {
  it("waits longer before each ask and stops after three", () => {
    expect([0, 1, 2].map(watchDelay)).toEqual([2_500, 5_000, 10_000]);
    expect(watchDelay(3)).toBeNull();
  });
});

describe("safely", () => {
  it("runs the step", () => {
    const step = vi.fn();
    safely("a step", step);
    expect(step).toHaveBeenCalledOnce();
  });

  it("reports a throw and returns, so the steps after it still run", () => {
    const report = vi.spyOn(console, "error").mockImplementation(() => {});
    const after = vi.fn();
    safely("marking envelopes", () => {
      throw new Error("boom");
    });
    after();
    expect(report).toHaveBeenCalledOnce();
    expect(String(report.mock.calls[0]?.[0])).toContain("marking envelopes");
    expect(after).toHaveBeenCalledOnce();
  });
});
