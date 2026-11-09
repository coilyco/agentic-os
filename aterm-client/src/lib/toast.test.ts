import { describe, expect, it } from "vitest";
import { toastFor, TOAST_MS } from "./toast";

const asking = { sessionId: "a", identity: "Imp-Dragonfly", kind: "asking" as const };
const page = { sessionId: "b", identity: "Whale-Dragonfly", kind: "artifact" as const };
const another = { sessionId: "c", identity: "Frog-Ox", kind: "asking" as const };

describe("toastFor", () => {
  it("names the one seat that is waiting", () => {
    expect(toastFor([asking], null)).toEqual({ sessionId: "a", count: 1, text: "Imp-Dragonfly is asking" });
    expect(toastFor([page], null)?.text).toBe("Whale-Dragonfly left you a page");
  });

  it("stacks several waiting seats into one toast with a count, opening the first", () => {
    expect(toastFor([asking, page, another], null)).toEqual({ sessionId: "a", count: 3, text: "Imp-Dragonfly is asking, and 2 more waiting" });
  });

  it("leaves out the seat already on screen, and says nothing when that is the only one", () => {
    expect(toastFor([asking, page], "a")).toEqual({ sessionId: "b", count: 1, text: "Whale-Dragonfly left you a page" });
    expect(toastFor([asking], "a")).toBeNull();
    expect(toastFor([], null)).toBeNull();
  });

  it("dismisses itself after a few seconds", () => {
    expect(TOAST_MS).toBeGreaterThanOrEqual(3_000);
    expect(TOAST_MS).toBeLessThanOrEqual(10_000);
  });
});
