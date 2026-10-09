import { describe, expect, it } from "vitest";
import { ESCAPE, HOLD_MS, INTERRUPT, showStop } from "./stop";

const base = { working: true, hasText: false, justSent: false, offline: false };

describe("showStop", () => {
  it("replaces Send while a seat works and the field is empty", () => {
    expect(showStop(base)).toBe(true);
  });

  it("stays Send for an idle seat, so a tap never sends a key nobody meant", () => {
    expect(showStop({ ...base, working: false })).toBe(false);
  });

  it("stays Send while there is a draft, so a tap cannot lose it", () => {
    expect(showStop({ ...base, hasText: true })).toBe(false);
  });

  it("gives the button to the Sent confirmation first, and not at all while the host is away", () => {
    expect(showStop({ ...base, justSent: true })).toBe(false);
    expect(showStop({ ...base, offline: true })).toBe(false);
  });
});

describe("the keys", () => {
  it("are Escape and Control-C, with a hold long enough to tell from a tap", () => {
    expect(ESCAPE).toBe("\u001b");
    expect(INTERRUPT).toBe("\u0003");
    expect(HOLD_MS).toBeGreaterThanOrEqual(500);
  });
});
