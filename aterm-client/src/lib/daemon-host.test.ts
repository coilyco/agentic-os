import { describe, expect, it } from "vitest";
import { daemonUrl, hostLabel, launchRefusal, toAsk, toMessage, toSession } from "./daemon-host";

describe("aterm.daemon.v1 mapping", () => {
  it("reads a ready session as idle and carries the draft flag", () => {
    const session = toSession({ name: "session-48c6d1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "zsh", ready: true, bracketed_paste: true, kai_drafting: true, pending: 2 });
    expect(session).toEqual({ id: "session-48c6d1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "zsh", state: "idle", pending: 2, drafting: true, paste: true, degraded: [] });
  });

  it("carries the startup steps a session launched without", () => {
    const view = { name: "s", role: "r", identity: "i", seat: "codex", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0, degraded: ["card", "telemetry"] };
    expect(toSession(view).degraded).toEqual(["card", "telemetry"]);
  });

  it("reads a session not at its prompt as working", () => {
    expect(toSession({ name: "s", role: "r", identity: "i", seat: "codex", ready: false, bracketed_paste: false, kai_drafting: false, pending: 0 }).state).toBe("working");
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

describe("toAsk", () => {
  it("fills the optional ask_choice fields", () => {
    expect(toAsk({ id: "a", session: "s", question: "Which?", options: [{ label: "One" }] })).toEqual({
      id: "a", session: "s", header: "", question: "Which?", options: [{ label: "One", description: "" }], allowOther: false, multi: false,
    });
  });
});

describe("daemonUrl", () => {
  it("dials the local daemon from the dev server", () => {
    expect(daemonUrl({ protocol: "http:", host: "localhost:5173" }, true)).toBe("ws://127.0.0.1:7419");
  });

  it("dials back to the origin that served a build, secure when the page is", () => {
    expect(daemonUrl({ protocol: "https:", host: "mac.example.ts.net" }, false)).toBe("wss://mac.example.ts.net/");
    expect(daemonUrl({ protocol: "http:", host: "127.0.0.1:7419" }, false)).toBe("ws://127.0.0.1:7419/");
  });

  it("lets an explicit address win", () => {
    expect(daemonUrl({ protocol: "https:", host: "x" }, false, "ws://127.0.0.1:7420")).toBe("ws://127.0.0.1:7420");
  });
});

describe("hostLabel", () => {
  it("names the machine from the tower, and says this Mac locally", () => {
    expect(hostLabel({ hostname: "kais-macbook-pro.example.ts.net" }, false)).toBe("kais-macbook-pro");
    expect(hostLabel({ hostname: "localhost" }, false)).toBe("this Mac");
  });
});
