import { describe, expect, it } from "vitest";
import { contextPercent, contextText, formatTokens } from "./context";

describe("formatTokens", () => {
  it("keeps two figures at every size", () => {
    expect(formatTokens(0)).toBe("0");
    expect(formatTokens(842)).toBe("842");
    expect(formatTokens(4260)).toBe("4.3k");
    expect(formatTokens(535_004)).toBe("535k");
    expect(formatTokens(1_250_000)).toBe("1.3M");
  });

  it("rolls 999,500 and over into millions rather than printing 1000k", () => {
    expect(formatTokens(999_499)).toBe("999k");
    expect(formatTokens(999_500)).toBe("1.0M");
  });
});

describe("contextText", () => {
  it("shows the count alone where the window is unknown", () => {
    const reading = { tokens: 535_004, source: "claude" };
    expect(contextPercent(reading)).toBeNull();
    expect(contextText(reading)).toBe("535k tokens");
  });

  it("adds the share and the window where the source knows it", () => {
    const reading = { tokens: 116_857, window: 258_400, source: "codex" };
    expect(contextPercent(reading)).toBe(45);
    expect(contextText(reading)).toBe("117k tokens // 45% of 258k");
  });

  it("says under 1% rather than 0% for a small count in a large window", () => {
    expect(contextText({ tokens: 4260, window: 1_000_000, source: "proxy" })).toBe("4.3k tokens // <1% of 1.0M");
  });

  it("does not hide a context past its window", () => {
    expect(contextPercent({ tokens: 300_000, window: 200_000, source: "proxy" })).toBe(150);
  });
});
