import { describe, expect, it } from "vitest";
import { closedWords, fullyIdle } from "./idle-close";
import type { Session } from "./protocol";

const idle: Session = { id: "a-b", role: "r", seat: "claude", identity: "Seat-1", state: "idle", pending: 0, drafting: false, paste: true, degraded: [], screen: "idle", background: 0, ref: "1a2b3c4d" };
const calm = { needsKai: false, unseen: false };

describe("fullyIdle", () => {
  it("is true for an idle seat with nothing running, nothing queued and nothing asked", () => {
    expect(fullyIdle(idle, calm)).toBe(true);
  });

  it("is false while the seat works, starts, or the daemon reads a prompt", () => {
    expect(fullyIdle({ ...idle, state: "working" }, calm)).toBe(false);
    expect(fullyIdle({ ...idle, starting: true }, calm)).toBe(false);
    expect(fullyIdle({ ...idle, screen: "busy" }, calm)).toBe(false);
    expect(fullyIdle({ ...idle, screen: "prompt" }, calm)).toBe(false);
  });

  it("is false while the seat waits on CI or a monitor", () => {
    expect(fullyIdle({ ...idle, background: 1 }, calm)).toBe(false);
  });

  it("is false when the daemon does not say, since absent is unknown", () => {
    const { background: _b, ...noBackground } = idle;
    const { screen: _s, ...noScreen } = idle;
    const { ref: _r, ...noRef } = idle;
    expect(fullyIdle(noBackground, calm)).toBe(false);
    expect(fullyIdle(noScreen, calm)).toBe(false);
    expect(fullyIdle(noRef, calm)).toBe(false);
  });

  it("is false with a queued message or a draft from Kai", () => {
    expect(fullyIdle({ ...idle, pending: 1 }, calm)).toBe(false);
    expect(fullyIdle({ ...idle, drafting: true }, calm)).toBe(false);
  });

  it("is false when a card or a waiting page needs Kai, or its reply is unread", () => {
    expect(fullyIdle(idle, { needsKai: true, unseen: false })).toBe(false);
    expect(fullyIdle(idle, { needsKai: false, unseen: true })).toBe(false);
  });
});

describe("closedWords", () => {
  it("names the seat and the one way back that exists", () => {
    expect(closedWords("Seat-1", "Frontend Engineer")).toBe("Closed Seat-1. It does not reopen from here. Start Frontend Engineer again from Start.");
  });
});
