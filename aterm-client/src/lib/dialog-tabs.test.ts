import { describe, expect, it } from "vitest";
import { parseTabBar, progressText, readTabs, tabKeys } from "./dialog-tabs";

// The tab bar of a three-question dialog, as Claude Code 2.1.318 draws it.
const BAR = "←  ☐ Accent  ☒ Routes  ☐ Name  ✔ Submit  →";
// The current tab's cells carry a background, from before its mark to past its label.
const highlight = (label: string): Set<number> => {
  const start = BAR.indexOf(label) - 3;
  return new Set(Array.from({ length: label.length + 4 }, (_, i) => start + i));
};

describe("parseTabBar", () => {
  it("reads each tab's label, whether it is answered, and which is Submit", () => {
    expect(parseTabBar(BAR)?.map((tab) => [tab.label, tab.answered, tab.submit])).toEqual([
      ["Accent", false, false],
      ["Routes", true, false],
      ["Name", false, false],
      ["Submit", false, true],
    ]);
  });

  it("is not a tab bar for a lone chip, a shell prompt, or ordinary text", () => {
    expect(parseTabBar(" ☐ Accent")).toBeNull();
    expect(parseTabBar("~/projects ❯ ls")).toBeNull();
    expect(parseTabBar("← back to the list")).toBeNull();
  });

  it("keeps a label with a space in it whole", () => {
    expect(parseTabBar("←  ☐ Which route  ✔ Submit  →")?.map((tab) => tab.label)).toEqual(["Which route", "Submit"]);
  });
});

describe("readTabs", () => {
  it("finds the current tab by the background on its cells", () => {
    expect(readTabs(BAR, highlight("Routes"))?.current).toBe(1);
    expect(readTabs(BAR, highlight("Accent"))?.current).toBe(0);
    expect(readTabs(BAR, highlight("Submit"))?.current).toBe(3);
  });

  it("does not guess when no tab is marked", () => {
    expect(readTabs(BAR, new Set())?.current).toBe(-1);
  });
});

describe("tabKeys", () => {
  const RIGHT = "\x1b[C";
  const LEFT = "\x1b[D";

  it("presses the arrow that many times from a tab it knows", () => {
    expect(tabKeys(0, 2, 4)).toEqual([RIGHT + RIGHT]);
    expect(tabKeys(3, 1, 4)).toEqual([LEFT + LEFT]);
    expect(tabKeys(2, 2, 4)).toEqual([]);
  });

  it("goes to the first tab and then across when it cannot tell where it is, which is safe since the ends stop", () => {
    expect(tabKeys(-1, 2, 4)).toEqual([LEFT.repeat(4), RIGHT + RIGHT]);
    expect(tabKeys(-1, 0, 4)).toEqual([LEFT.repeat(4)]);
  });
});

describe("progressText", () => {
  it("says which question of how many, not counting Submit", () => {
    expect(progressText(readTabs(BAR, highlight("Routes"))!)).toBe("Question 2 of 3");
  });

  it("calls the Submit tab the review", () => {
    expect(progressText(readTabs(BAR, highlight("Submit"))!)).toBe("Review");
  });

  it("says nothing when the current tab is unknown", () => {
    expect(progressText(readTabs(BAR, new Set())!)).toBe("");
  });
});
