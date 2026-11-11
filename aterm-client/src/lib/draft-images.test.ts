import { describe, expect, it } from "vitest";
import { KEEP_MS } from "./draft-store";
import { MAX_BYTES, planPrune } from "./draft-images";

const NOW = 1_000_000_000;
const row = (key: string, ago: number, bytes: number) => ({ key, at: NOW - ago, bytes });

describe("planPrune", () => {
  it("drops nothing while everything is fresh and fits", () => {
    expect(planPrune([row("a", 1000, 100), row("b", 2000, 100)], "a", NOW)).toEqual([]);
  });

  it("drops a draft's images older than a day, since its seat name may be another seat's", () => {
    expect(planPrune([row("old", KEEP_MS + 1, 100), row("new", 1000, 100)], "new", NOW)).toEqual(["old"]);
  });

  it("drops the oldest first until the rest fit the budget", () => {
    const rows = [row("oldest", 3000, MAX_BYTES / 2), row("middle", 2000, MAX_BYTES / 2), row("newest", 1000, MAX_BYTES / 2)];
    expect(planPrune(rows, "newest", NOW)).toEqual(["oldest"]);
  });

  it("never drops the draft being saved, even when it is the oldest", () => {
    const rows = [row("mine", 9000, MAX_BYTES - 10), row("other", 1000, 100)];
    expect(planPrune(rows, "mine", NOW)).toEqual(["other"]);
  });
});
