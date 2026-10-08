import { describe, expect, it } from "vitest";
import { BACKOFF_CAP_MS, backoffDelay, differentBuild, secondsUntil } from "./backoff";

const middle = () => 0.5;

describe("backoffDelay", () => {
  it("doubles from one second, then holds at the cap", () => {
    const schedule = [0, 1, 2, 3, 4, 5, 6, 20].map((attempt) => backoffDelay(attempt, middle));
    expect(schedule).toEqual([1000, 2000, 4000, 8000, 16000, 30000, 30000, 30000]);
  });

  it("spreads each wait by 20% either way, and never past the cap", () => {
    expect(backoffDelay(2, () => 0)).toBe(3200);
    expect(backoffDelay(2, () => 1)).toBe(4800);
    expect(backoffDelay(9, () => 1)).toBe(BACKOFF_CAP_MS);
    expect(backoffDelay(9, () => 0)).toBe(24000);
  });

  it("survives an attempt count nobody would reach", () => {
    expect(backoffDelay(10_000, middle)).toBe(BACKOFF_CAP_MS);
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
