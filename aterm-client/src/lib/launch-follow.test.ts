import { describe, expect, it } from "vitest";
import { beginLaunch, LAUNCH_WATCH_MS, newSeatFor } from "./launch-follow";

const seat = (id: string, role: string, state = "idle") => ({ id, role, state });

describe("newSeatFor", () => {
  const pending = beginLaunch("frontend-eng", [{ id: "frontend-eng-imp" }, { id: "eng-platform-beetle" }], 1000);

  it("opens the new session of the launched role", () => {
    const listed = [seat("frontend-eng-imp", "frontend-eng"), seat("frontend-eng-new", "frontend-eng"), seat("eng-platform-beetle", "eng-platform")];
    expect(newSeatFor(pending, listed, 1500)).toBe("frontend-eng-new");
  });

  it("waits while the listing holds only seats it had", () => {
    expect(newSeatFor(pending, [seat("frontend-eng-imp", "frontend-eng")], 1500)).toBeNull();
  });

  it("ignores a new seat of another role", () => {
    expect(newSeatFor(pending, [seat("frontend-eng-imp", "frontend-eng"), seat("sysadmin-senior-new", "sysadmin-senior")], 1500)).toBeNull();
  });

  it("ignores a failed session, which is the launch's own report", () => {
    expect(newSeatFor(pending, [seat("frontend-eng-bad", "frontend-eng", "failed")], 1500)).toBeNull();
  });

  it("opens the first of two new sessions, in the host's order", () => {
    expect(newSeatFor(pending, [seat("frontend-eng-a", "frontend-eng"), seat("frontend-eng-b", "frontend-eng")], 1500)).toBe("frontend-eng-a");
  });

  it("gives up after the watch window and leaves the seat in the list", () => {
    const listed = [seat("frontend-eng-new", "frontend-eng")];
    expect(newSeatFor(pending, listed, 1000 + LAUNCH_WATCH_MS)).toBe("frontend-eng-new");
    expect(newSeatFor(pending, listed, 1001 + LAUNCH_WATCH_MS)).toBeNull();
  });

  it("starts from an empty roster", () => {
    expect(newSeatFor(beginLaunch("frontend-eng", [], 0), [seat("frontend-eng-new", "frontend-eng")], 10)).toBe("frontend-eng-new");
  });
});
