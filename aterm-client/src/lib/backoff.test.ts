import { describe, expect, it } from "vitest";
import { RETRY_SPAN_MS, RETRY_WAITS_MS, backoffDelay, differentBuild, secondsUntil } from "./backoff";

const middle = () => 0.5;

describe("backoffDelay", () => {
  it("doubles from one second", () => {
    expect([0, 1, 2, 3, 4, 5].map((attempt) => backoffDelay(attempt, middle))).toEqual([1000, 2000, 4000, 8000, 16000, 32000]);
  });

  it("lands the last automatic try one hour after the loss", () => {
    const waits: number[] = [];
    for (let attempt = 0; backoffDelay(attempt, middle) !== null; attempt++) waits.push(backoffDelay(attempt, middle)!);
    expect(waits.reduce((total, wait) => total + wait, 0)).toBe(RETRY_SPAN_MS);
    // 1 s doubling to about 17 minutes, then one trimmed wait that ends on the hour.
    expect(waits).toHaveLength(12);
    expect(waits.at(-2)).toBe(1_024_000);
    expect(waits.at(-1)).toBe(1_553_000);
  });

  it("keeps the last try within the jitter of the hour", () => {
    const sum = (random: () => number) => RETRY_WAITS_MS.reduce((total, _wait, attempt) => total + backoffDelay(attempt, random)!, 0);
    expect(sum(() => 0)).toBeCloseTo(RETRY_SPAN_MS * 0.8, -3);
    expect(sum(() => 1)).toBeCloseTo(RETRY_SPAN_MS * 1.2, -3);
  });

  it("spreads each wait by 20% either way", () => {
    expect(backoffDelay(2, () => 0)).toBe(3200);
    expect(backoffDelay(2, () => 1)).toBe(4800);
  });

  it("stops when the hour is spent, however many tries were counted", () => {
    expect(backoffDelay(RETRY_WAITS_MS.length, middle)).toBeNull();
    expect(backoffDelay(10_000, middle)).toBeNull();
  });
});

describe("secondsUntil", () => {
  it("rounds up so the countdown never reads zero before the dial", () => {
    expect(secondsUntil(5000, 1000)).toBe(4);
    expect(secondsUntil(5000, 4001)).toBe(1);
    expect(secondsUntil(5000, 5000)).toBe(0);
  });

  it("does not count below zero when the dial is late", () => {
    expect(secondsUntil(5000, 9000)).toBe(0);
  });
});

describe("differentBuild", () => {
  it("is quiet when the same build answers, or either build is unknown", () => {
    expect(differentBuild("0.443.0", "0.443.0")).toBe(false);
    expect(differentBuild(undefined, "0.444.0")).toBe(false);
    expect(differentBuild("0.443.0", undefined)).toBe(false);
  });

  it("offers a reload for a higher release, comparing by number and not by text", () => {
    expect(differentBuild("0.443.0", "0.444.0")).toBe(true);
    expect(differentBuild("0.99.0", "0.100.0")).toBe(true);
    expect(differentBuild("v0.443.9", "0.444.0")).toBe(true);
  });

  it("does not call a rollback newer", () => {
    expect(differentBuild("0.444.0", "0.443.0")).toBe(false);
    expect(differentBuild("1.0.0", "0.999.9")).toBe(false);
  });

  it("counts a build it cannot order as different, since the page cannot tell which is newer", () => {
    expect(differentBuild("dev", "0.444.0")).toBe(true);
    expect(differentBuild("0.444.0", "dev")).toBe(true);
  });
});
