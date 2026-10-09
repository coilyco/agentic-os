import { describe, expect, it } from "vitest";
import { AWAY_MS, REFUSAL_WINDOW_MS, restoreRefused, worthJumping } from "./drafts";

describe("restoreRefused", () => {
  it("puts a refused send back in the draft, ahead of what was typed since", () => {
    expect(restoreRefused("", { body: "ship it", at: 1000 }, 1500)).toEqual({ draft: "ship it", restored: true });
    expect(restoreRefused("and then", { body: "ship it", at: 1000 }, 1500)).toEqual({ draft: "ship it\nand then", restored: true });
  });

  it("leaves the draft alone when nothing was just sent, or the lock is too late to be that send", () => {
    expect(restoreRefused("typing", null, 1500)).toEqual({ draft: "typing", restored: false });
    expect(restoreRefused("", { body: "old", at: 0 }, REFUSAL_WINDOW_MS + 1)).toEqual({ draft: "", restored: false });
  });
});

describe("worthJumping", () => {
  it("jumps for someone who was away, as alt-tab does", () => {
    expect(worthJumping(1000, 1000 + AWAY_MS, false)).toBe(true);
  });

  it("does not jump for a blur that ends at once, as a soft keyboard or a paste menu causes", () => {
    expect(worthJumping(1000, 1000 + AWAY_MS - 1, false)).toBe(false);
  });

  it("does not jump when the page was never left, or a draft is waiting", () => {
    expect(worthJumping(null, 99999, false)).toBe(false);
    expect(worthJumping(1000, 99999, true)).toBe(false);
  });
});
