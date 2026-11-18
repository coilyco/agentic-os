import { describe, expect, it } from "vitest";
import { MAX_BYTES, PER_SEAT_SENDS, planKeep, rowKey } from "./history-images";
import { KEEP_MS } from "./send-history";

const NOW = 1_000_000_000_000;
const row = (seat: string, id: string, at: number, bytes = 1000) => ({ key: rowKey(seat, id), seat, at, bytes });

describe("which stored images go", () => {
  it("keeps the newest ten per seat and drops the rest", () => {
    const rows = Array.from({ length: PER_SEAT_SENDS + 3 }, (_, i) => row("mac/imp", `s${i}`, NOW + i));
    const gone = planKeep(rows, NOW + 100);
    expect(gone).toHaveLength(3);
    expect(gone).toEqual(expect.arrayContaining([rowKey("mac/imp", "s0"), rowKey("mac/imp", "s1"), rowKey("mac/imp", "s2")]));
  });

  it("counts each seat on its own", () => {
    const rows = [...Array.from({ length: 6 }, (_, i) => row("mac/a", `s${i}`, NOW + i)), ...Array.from({ length: 6 }, (_, i) => row("mac/b", `s${i}`, NOW + i))];
    expect(planKeep(rows, NOW + 100)).toEqual([]);
  });

  it("drops what is past the keep time", () => {
    expect(planKeep([row("mac/imp", "old", NOW - KEEP_MS - 1), row("mac/imp", "new", NOW)], NOW)).toEqual([rowKey("mac/imp", "old")]);
  });

  it("drops the oldest across seats until the rest fit the byte budget", () => {
    const big = MAX_BYTES / 2 + 1;
    const rows = [row("mac/a", "x", NOW, big), row("mac/b", "y", NOW + 1, big), row("mac/c", "z", NOW + 2, 10)];
    expect(planKeep(rows, NOW + 5)).toEqual([rowKey("mac/a", "x")]);
  });
});
