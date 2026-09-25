import { describe, expect, it } from "vitest";
import { launchRefusal, toMessage, toSession } from "./daemon-host";

describe("aterm.daemon.v1 mapping", () => {
  it("reads a ready session as idle and carries the draft flag", () => {
    const session = toSession({ name: "session-48c6d1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "zsh", ready: true, kai_drafting: true, pending: 2 });
    expect(session).toEqual({ id: "session-48c6d1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "zsh", state: "idle", pending: 2, drafting: true });
  });

  it("reads a session not at its prompt as working", () => {
    expect(toSession({ name: "s", role: "r", identity: "i", seat: "codex", ready: false, kai_drafting: false, pending: 0 }).state).toBe("working");
  });

  it("splits the sender and keeps the failure reason", () => {
    const message = toMessage({ id: "a1", from: "eng-platform Beetle-Ox", target: "game-dev", state: "failed", reason: "no live session" });
    expect(message).toEqual({ id: "a1", from: { role: "eng-platform", identity: "Beetle-Ox" }, target: "game-dev", session: null, state: "failed", reason: "no live session" });
  });
});

describe("launchRefusal", () => {
  it("names a refused slug or seat apart from a launch that failed on the host", () => {
    expect(launchRefusal(2, "launch needs a role slug")).toMatch(/would not launch/);
    expect(launchRefusal(5, "exit status 4")).toMatch(/failed on the host: exit status 4/);
  });
});
