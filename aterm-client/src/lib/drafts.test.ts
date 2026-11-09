import { describe, expect, it } from "vitest";
import { REFUSAL_WINDOW_MS, restoreRefused } from "./drafts";

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
