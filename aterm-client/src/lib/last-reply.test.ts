import { describe, expect, it } from "vitest";
import { pinnedLine, replyAge, replyKey } from "./last-reply";

describe("replyAge", () => {
  const now = 1_000_000_000_000;
  it("says nothing without a time", () => expect(replyAge(undefined, now)).toBe(""));
  it("says just now under ten seconds", () => expect(replyAge(now - 4000, now)).toBe("just now"));
  it("counts seconds, minutes, hours and days", () => {
    expect(replyAge(now - 42_000, now)).toBe("42s ago");
    expect(replyAge(now - 3 * 60_000, now)).toBe("3m ago");
    expect(replyAge(now - 5 * 3_600_000, now)).toBe("5h ago");
    expect(replyAge(now - 2 * 86_400_000, now)).toBe("2d ago");
  });
  it("does not go negative when the clock is behind", () => expect(replyAge(now + 60_000, now)).toBe("just now"));
});

describe("pinnedLine", () => {
  it("folds whitespace into one sentence", () => expect(pinnedLine("  I put an\n access   sysadmin on it. ")).toBe("I put an access sysadmin on it."));
  it("is empty without a reply", () => expect(pinnedLine(undefined)).toBe(""));
});

describe("replyKey", () => {
  it("changes when the reply's time or first line does", () => {
    expect(replyKey("a", 1, "x")).not.toBe(replyKey("a", 2, "x"));
    expect(replyKey("a", 1, "x")).not.toBe(replyKey("a", 1, "y"));
    expect(replyKey("a", 1, "x")).toBe(replyKey("a", 1, "x"));
  });
});
