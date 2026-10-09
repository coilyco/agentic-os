import { describe, expect, it } from "vitest";
import { toastFor, TOAST_MS } from "./toast";

const asking = { sessionId: "a", identity: "Imp-Dragonfly", kind: "asking" as const };
const done = { sessionId: "b", identity: "Whale-Dragonfly", kind: "done" as const };
const another = { sessionId: "c", identity: "Frog-Ox", kind: "asking" as const };

describe("toastFor", () => {
  it("names the one seat that is waiting", () => {
    expect(toastFor([asking], null)).toEqual({ sessionId: "a", count: 1, text: "Imp-Dragonfly is asking" });
    expect(toastFor([done], null)?.text).toBe("Whale-Dragonfly is done");
  });

  it("stacks several waiting seats into one toast with a count, opening the first", () => {
    expect(toastFor([asking, done, another], null)).toEqual({ sessionId: "a", count: 3, text: "Imp-Dragonfly is asking, and 2 more waiting" });
  });

  it("leaves out the seat already on screen, and says nothing when that is the only one", () => {
    expect(toastFor([asking, done], "a")).toEqual({ sessionId: "b", count: 1, text: "Whale-Dragonfly is done" });
    expect(toastFor([asking], "a")).toBeNull();
    expect(toastFor([], null)).toBeNull();
  });

  it("dismisses itself after a few seconds", () => {
    expect(TOAST_MS).toBeGreaterThanOrEqual(3_000);
    expect(TOAST_MS).toBeLessThanOrEqual(10_000);
  });
});
