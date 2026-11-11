import { describe, expect, it } from "vitest";
import { KEEP_MS, MAX_DRAFTS } from "./draft-store";
import { newerWords, planWordPrune } from "./draft-words";

const NOW = 1_000_000_000;

describe("planWordPrune", () => {
  it("drops a copy older than a day and keeps the rest", () => {
    const rows = [{ key: "old", at: NOW - KEEP_MS - 1 }, { key: "fresh", at: NOW - KEEP_MS }];
    expect(planWordPrune(rows, NOW)).toEqual(["old"]);
  });

  it("drops the oldest past the cap", () => {
    const rows = Array.from({ length: MAX_DRAFTS + 2 }, (_, i) => ({ key: `k${i}`, at: NOW - i }));
    expect(planWordPrune(rows, NOW)).toEqual([`k${MAX_DRAFTS}`, `k${MAX_DRAFTS + 1}`]);
  });
});

describe("newerWords", () => {
  const mirrored = (at: number, text = "mirror") => ({ key: "h/s", text, at });

  it("brings back the mirror when local storage lost the draft", () => {
    expect(newerWords(undefined, mirrored(NOW))).toBe("mirror");
  });

  it("brings back the mirror when it is newer than the stored copy", () => {
    expect(newerWords({ text: "stored", at: NOW - 1 }, mirrored(NOW))).toBe("mirror");
  });

  it("leaves the stored copy when it is as new or newer", () => {
    expect(newerWords({ text: "stored", at: NOW }, mirrored(NOW))).toBeUndefined();
    expect(newerWords({ text: "stored", at: NOW + 1 }, mirrored(NOW))).toBeUndefined();
  });

  it("brings back nothing when the box was emptied after the mirror was written", () => {
    expect(newerWords(undefined, mirrored(NOW), NOW)).toBeUndefined();
    expect(newerWords(undefined, mirrored(NOW), NOW + 1)).toBeUndefined();
    expect(newerWords(undefined, mirrored(NOW + 1), NOW)).toBe("mirror");
  });

  it("brings back nothing when there is no mirror or it is empty", () => {
    expect(newerWords({ text: "stored", at: NOW }, undefined)).toBeUndefined();
    expect(newerWords(undefined, mirrored(NOW, ""))).toBeUndefined();
  });
});
