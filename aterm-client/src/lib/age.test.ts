import { describe, expect, it } from "vitest";
import { ageText, quietSince, stateWithAge } from "./age";

describe("ageText", () => {
  it("says nothing for a moment that is not worth a number", () => {
    expect(ageText(0)).toBe("");
    expect(ageText(9)).toBe("");
  });

  it("counts seconds, minutes, hours and days, rounding down", () => {
    expect(ageText(10)).toBe("10s");
    expect(ageText(59)).toBe("59s");
    expect(ageText(60)).toBe("1m");
    expect(ageText(725)).toBe("12m");
    expect(ageText(3600)).toBe("1h");
    expect(ageText(7300)).toBe("2h");
    expect(ageText(90000)).toBe("1d");
  });
});

describe("quietSince", () => {
  const received = 1_000_000;

  it("backdates the daemon's reading from when it arrived", () => {
    expect(quietSince(120, received, undefined)).toBe(received - 120_000);
  });

  it("moves forward when this client saw the seat write after the reading", () => {
    expect(quietSince(120, received, received + 5_000)).toBe(received + 5_000);
  });

  it("ignores output older than the reading, which the reading already counts", () => {
    expect(quietSince(120, received, received - 500_000)).toBe(received - 120_000);
  });

  it("uses what this client saw when the daemon sent no reading, and nothing otherwise", () => {
    expect(quietSince(undefined, received, received + 1_000)).toBe(received + 1_000);
    expect(quietSince(undefined, received, undefined)).toBeUndefined();
  });
});

describe("stateWithAge", () => {
  const now = 10_000_000;
  const quiet = (seconds: number) => now - seconds * 1000;

  it("says how long an idle seat has been quiet", () => {
    expect(stateWithAge("idle", quiet(725), now)).toBe("idle 12m");
  });

  it("leaves a fresh idle seat alone", () => {
    expect(stateWithAge("idle", quiet(4), now)).toBe("idle");
  });

  it("flags a working seat that has gone silent, as a way to spot a stuck one", () => {
    expect(stateWithAge("working", quiet(190), now)).toBe("working, quiet 3m");
  });

  it("leaves a working seat that is writing alone", () => {
    expect(stateWithAge("working", quiet(12), now)).toBe("working");
  });

  it("says how long someone has been waiting on a finished seat", () => {
    expect(stateWithAge("done, your turn", quiet(180), now)).toBe("done, your turn 3m");
  });

  it("changes nothing without a reading", () => {
    expect(stateWithAge("idle", undefined, now)).toBe("idle");
  });
});
