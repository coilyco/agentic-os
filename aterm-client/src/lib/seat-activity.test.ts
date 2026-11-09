import { describe, expect, it } from "vitest";
import { toSession } from "./daemon-host";
import { activityFlag, activityWords, STALE_AFTER_S, toActivity } from "./seat-activity";

const NOW = Date.parse("2026-11-05T12:00:00Z");
const view = { name: "a-b", role: "a", identity: "Imp", seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0 };

describe("toActivity", () => {
  it("reads the text and the time the seat set it", () => {
    expect(toActivity("Fixing the sheet", "2026-11-05T11:59:00Z")).toEqual({ text: "Fixing the sheet", at: Date.parse("2026-11-05T11:59:00Z") });
  });

  it("has no line without text, which is how a seat clears it", () => {
    expect(toActivity(undefined, "2026-11-05T11:59:00Z")).toBeUndefined();
    expect(toActivity("", "2026-11-05T11:59:00Z")).toBeUndefined();
    expect(toActivity("   ", "2026-11-05T11:59:00Z")).toBeUndefined();
  });

  it("has no line when the time is missing or unreadable, since its age could not be said", () => {
    expect(toActivity("Fixing the sheet", undefined)).toBeUndefined();
    expect(toActivity("Fixing the sheet", "yesterday")).toBeUndefined();
  });
});

describe("activityFlag", () => {
  const at = (secondsAgo: number) => ({ text: "x", at: NOW - secondsAgo * 1000 });

  it("is empty while the line is recent", () => {
    expect(activityFlag(at(0), NOW)).toBe("");
    expect(activityFlag(at(STALE_AFTER_S - 1), NOW)).toBe("");
  });

  it("says how old the line is once it is stale", () => {
    expect(activityFlag(at(STALE_AFTER_S), NOW)).toBe("5m ago");
    expect(activityFlag(at(3 * 3600), NOW)).toBe("3h ago");
    expect(activityFlag(at(2 * 86400), NOW)).toBe("2d ago");
  });

  it("does not go negative when the daemon's clock runs ahead", () => {
    expect(activityFlag({ text: "x", at: NOW + 60_000 }, NOW)).toBe("");
  });
});

describe("activityWords", () => {
  it("is empty with no line", () => {
    expect(activityWords(undefined, NOW)).toBe("");
  });

  it("names the line, and its age only when stale", () => {
    expect(activityWords({ text: "Fixing the sheet", at: NOW - 30_000 }, NOW)).toBe("says Fixing the sheet");
    expect(activityWords({ text: "Fixing the sheet", at: NOW - 3 * 3600_000 }, NOW)).toBe("says Fixing the sheet, set 3h ago");
  });
});

describe("toSession", () => {
  it("carries the activity fields of a daemon that sends them", () => {
    expect(toSession({ ...view, activity: "Fixing the sheet", activity_at: "2026-11-05T11:59:00Z" }).activity?.text).toBe("Fixing the sheet");
  });

  it("leaves the row unchanged for a daemon that sends none", () => {
    expect("activity" in toSession(view)).toBe(false);
  });
});

describe("a line set before the seat's present state", () => {
  const line = { text: "Running the gate", at: 1_000_000 };

  it("reads as old even while recent, and says how old", () => {
    expect(activityFlag(line, line.at + 120_000, line.at + 60_000)).toBe("2m ago");
  });

  it("stays current when the state began as the line was set", () => {
    expect(activityFlag(line, line.at + 120_000, line.at + 5_000)).toBe("");
    expect(activityFlag(line, line.at + 120_000, line.at - 60_000)).toBe("");
  });

  it("changes nothing when the daemon sends no state age", () => {
    expect(activityFlag(line, line.at + 120_000)).toBe("");
    expect(activityWords(line, line.at + 120_000, line.at + 60_000)).toBe("says Running the gate, set 2m ago");
  });
});
