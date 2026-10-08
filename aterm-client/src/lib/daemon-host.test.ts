import { afterEach, describe, expect, it, vi } from "vitest";
import type { HostEvent } from "./protocol";
import { DaemonHost, daemonUrl, hostLabel, launchRefusal, toAsk, toMessage, toSession } from "./daemon-host";

describe("aterm.daemon.v1 mapping", () => {
  it("reads a ready session as idle and carries the draft flag", () => {
    const session = toSession({ name: "session-48c6d1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "zsh", ready: true, bracketed_paste: true, kai_drafting: true, pending: 2 });
    expect(session).toEqual({ id: "session-48c6d1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "zsh", state: "idle", pending: 2, drafting: true, paste: true, degraded: [] });
  });

  it("carries the startup steps a session launched without", () => {
    const view = { name: "s", role: "r", identity: "i", seat: "codex", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0, degraded: ["card", "telemetry"] };
    expect(toSession(view).degraded).toEqual(["card", "telemetry"]);
  });

  it("carries a context reading, and drops an absent window instead of showing zero", () => {
    const base = { name: "s", role: "r", identity: "i", seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0 };
    expect(toSession({ ...base, context: { tokens: 1050, source: "claude" } }).context).toEqual({ tokens: 1050, source: "claude" });
    expect(toSession({ ...base, context: { tokens: 5000, window: 258400, source: "codex" } }).context).toEqual({ tokens: 5000, window: 258400, source: "codex" });
    expect("context" in toSession(base)).toBe(false);
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

// Only what DaemonHost touches: a frame in becomes `message`, a frame out is recorded.
class StubSocket {
  static last: StubSocket;
  sent: string[] = [];
  private handlers = new Map<string, ((event: unknown) => void)[]>();
  constructor() {
    StubSocket.last = this;
  }
  addEventListener(type: string, handler: (event: unknown) => void): void {
    this.handlers.set(type, [...(this.handlers.get(type) ?? []), handler]);
  }
  send(line: string): void {
    this.sent.push(line);
  }
  close(): void {}
  receive(frame: Record<string, unknown>): void {
    for (const handler of this.handlers.get("message") ?? []) handler({ data: JSON.stringify(frame) });
  }
}

function connect(): { host: DaemonHost; events: HostEvent[]; socket: StubSocket } {
  vi.stubGlobal("WebSocket", StubSocket);
  const host = new DaemonHost("ws://stub");
  const events: HostEvent[] = [];
  host.subscribe((event) => events.push(event));
  return { host, events, socket: StubSocket.last };
}

const typings = (events: HostEvent[]) => events.filter((event) => event.type === "typing");

describe("the typing guard", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("reads read-only from the welcome, before anything is typed", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", typing: { allowed: false, reason: "session_descendant" } });
    expect(typings(events)).toEqual([{ type: "typing", typing: { allowed: false, reason: "session_descendant" } }]);
    host.close();
  });

  it("stays quiet when a daemon predates the guard, and replays the state to a later subscriber", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1" });
    expect(typings(events)).toEqual([]);
    socket.receive({ type: "error", id: "c9", code: 6, error: "input refused", reason: "peer_unread" });
    const late: HostEvent[] = [];
    host.subscribe((event) => late.push(event));
    expect(typings(late)).toEqual([{ type: "typing", typing: { allowed: false, reason: "peer_unread" } }]);
    host.close();
  });

  it("turns a refused input into read-only without a notice, and an unrelated error into a notice only", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", typing: { allowed: true } });
    socket.receive({ type: "error", id: "c5", code: 6, error: "input refused", reason: "session_descendant" });
    socket.receive({ type: "error", id: "c6", code: 3, error: "the ask is gone", reason: "settled" });
    expect(typings(events).at(-1)).toEqual({ type: "typing", typing: { allowed: false, reason: "session_descendant" } });
    expect(events.filter((event) => event.type === "notice")).toEqual([{ type: "notice", text: "the ask is gone" }]);
    host.close();
  });

  it("fails a launch the guard refused with the reason, not as a launch that broke on the host", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1" });
    host.launch("frontend-eng", "claude");
    const id = JSON.parse(socket.sent.at(-1) ?? "{}").id;
    socket.receive({ type: "error", id, code: 6, error: "launch refused", reason: "session_descendant" });
    const failed = events.filter((event) => event.type === "launch" && event.state === "failed");
    expect(failed).toHaveLength(1);
    expect(failed[0]).toMatchObject({ role: "frontend-eng", text: expect.stringMatching(/started by an agent session/) });
    host.close();
  });
});
