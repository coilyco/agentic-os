import { describe, expect, it } from "vitest";
import { toDeadSeats } from "./daemon-host";
import { deadDetail } from "./dead-seats";

describe("dead seats", () => {
  it("reads the daemon's list, absent meaning none", () => {
    expect(toDeadSeats(undefined)).toEqual([]);
    expect(toDeadSeats([{ name: "eng-platform-wise-beetle", ref: "0a1b2c3d", role: "eng-platform", seat: "claude", ended: "2026-11-27T12:00:00Z" }])).toEqual([
      { name: "eng-platform-wise-beetle", ref: "0a1b2c3d", role: "eng-platform", identity: "eng-platform-wise-beetle", seat: "claude", endedAt: Date.parse("2026-11-27T12:00:00Z") },
    ]);
  });

  it("says the role and how long ago the holder was found gone", () => {
    const found = Date.parse("2026-11-27T12:00:00Z");
    expect(deadDetail("Platform Engineer", found, found + 3 * 3600_000)).toBe("Platform Engineer // found dead 3h ago");
    expect(deadDetail("Platform Engineer", found, found + 2000)).toBe("Platform Engineer // found dead just now");
  });
});
