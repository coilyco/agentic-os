import { afterEach, describe, expect, it, vi } from "vitest";
import { DaemonHost, terminalRefusal } from "./daemon-host";
import type { HostEvent } from "./protocol";

// The stub from daemon-host.test.ts: a frame in becomes `message`, a frame out is kept.
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
  lastSent(): Record<string, unknown> {
    return JSON.parse(this.sent.at(-1) ?? "{}");
  }
}

function connect(features?: string[]) {
  vi.stubGlobal("WebSocket", StubSocket);
  const host = new DaemonHost("ws://stub");
  const events: HostEvent[] = [];
  host.subscribe((event) => events.push(event));
  const socket = StubSocket.last;
  socket.receive({ type: "welcome", format: "aterm.daemon.v1", ...(features ? { features } : {}) });
  return { host, events, socket };
}

const ofType = (events: HostEvent[], type: HostEvent["type"]) => events.filter((event) => event.type === type);

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("the terminal feature", () => {
  it("reads it from the welcome, so the pane shows only on a daemon that has it", () => {
    const { host, events } = connect(["asks", "terminals"]);
    expect(ofType(events, "features")).toEqual([{ type: "features", terminals: true }]);
    host.close();
  });

  it("reads its absence as a daemon that predates the terminal, with or without a feature list", () => {
    for (const features of [undefined, ["asks"]]) {
      const { host, events } = connect(features);
      expect(ofType(events, "features")).toEqual([{ type: "features", terminals: false }]);
      host.close();
    }
  });

  it("replays it to a subscriber that arrives after the welcome", () => {
    const { host } = connect(["terminals"]);
    const late: HostEvent[] = [];
    host.subscribe((event) => late.push(event));
    expect(ofType(late, "features")).toEqual([{ type: "features", terminals: true }]);
    host.close();
  });
});

describe("opening and closing a terminal", () => {
  it("spawns the terminal kind with nothing a terminal refuses, and names the shell from the reply", () => {
    const { host, events, socket } = connect(["terminals"]);
    host.terminals.open("sysadmin-senior-turtle-ox");
    const sent = socket.lastSent();
    expect(sent).toMatchObject({ type: "spawn", kind: "terminal" });
    for (const refused of ["session", "role", "identity", "seat", "argv", "env"]) expect(sent).not.toHaveProperty(refused);
    socket.receive({ type: "spawned", id: sent.id, session: "terminal-3f9a1c", pid: 123, kind: "terminal" });
    expect(ofType(events, "terminal_opened")).toEqual([{ type: "terminal_opened", label: "sysadmin-senior-turtle-ox", id: "terminal-3f9a1c" }]);
    host.close();
  });

  it("sends the seat as the spawn's label only to a daemon that keeps one", () => {
    const labelled = connect(["terminals", "terminal-label"]);
    labelled.host.terminals.open("sysadmin-senior-turtle-ox");
    expect(labelled.socket.lastSent()).toMatchObject({ type: "spawn", kind: "terminal", label: "sysadmin-senior-turtle-ox" });
    labelled.host.close();
    const plain = connect(["terminals"]);
    plain.host.terminals.open("sysadmin-senior-turtle-ox");
    expect(plain.socket.lastSent()).not.toHaveProperty("label");
    plain.host.close();
  });

  it("closes by name with the close frame's target", () => {
    const { host, socket } = connect(["terminals"]);
    host.terminals.close("terminal-3f9a1c");
    expect(socket.lastSent()).toMatchObject({ type: "close", target: "terminal-3f9a1c" });
    host.close();
  });

  it("does not take a seat's own spawn reply for a terminal", () => {
    const { host, events, socket } = connect(["terminals"]);
    socket.receive({ type: "spawned", id: "other", session: "frontend-eng-imp-dragonfly", pid: 9 });
    expect(ofType(events, "terminal_opened")).toEqual([]);
    host.close();
  });
});

describe("the terminal list", () => {
  it("reads the terminals beside the sessions, and an absent list as none", () => {
    const { host, events, socket } = connect(["terminals"]);
    socket.receive({ type: "sessions", sessions: [], terminals: [{ name: "terminal-3f9a1c", pid: 1, started: "t", clients: 1, cwd: "/h" }] });
    socket.receive({ type: "sessions", sessions: [] });
    expect(ofType(events, "terminals")).toEqual([
      { type: "terminals", terminals: [{ id: "terminal-3f9a1c" }] },
      { type: "terminals", terminals: [] },
    ]);
    host.close();
  });

  it("carries the label the daemon lists, and none for a shell without one", () => {
    const { host, events, socket } = connect(["terminals", "terminal-label"]);
    socket.receive({ type: "sessions", sessions: [], terminals: [{ name: "terminal-3f9a1c", label: "sysadmin-senior-turtle-ox" }, { name: "terminal-b2" }] });
    expect(ofType(events, "terminals")).toEqual([
      { type: "terminals", terminals: [{ id: "terminal-3f9a1c", label: "sysadmin-senior-turtle-ox" }, { id: "terminal-b2" }] },
    ]);
    host.close();
  });

  it("never lists a terminal as a seat, and never watches one for busyness", () => {
    const { host, events, socket } = connect(["terminals"]);
    socket.receive({ type: "sessions", sessions: [], terminals: [{ name: "terminal-3f9a1c" }] });
    expect(ofType(events, "sessions").every((event) => event.type === "sessions" && event.sessions.length === 0)).toBe(true);
    expect(socket.sent.some((line) => JSON.parse(line).type === "attach")).toBe(false);
    host.close();
  });

  it("does not turn its output into a seat going busy, which would republish the roster", () => {
    const { host, events, socket } = connect(["terminals"]);
    socket.receive({ type: "sessions", sessions: [], terminals: [{ name: "terminal-3f9a1c" }] });
    const before = ofType(events, "sessions").length;
    socket.receive({ type: "output", session: "terminal-3f9a1c", data: btoa("hi") });
    expect(ofType(events, "sessions")).toHaveLength(before);
    expect(ofType(events, "output")).toHaveLength(1);
    host.close();
  });
});

describe("a shell ending", () => {
  it("reports the code for a terminal, from the exit frame or from an attach to an ended one", () => {
    const { host, events, socket } = connect(["terminals"]);
    socket.receive({ type: "sessions", sessions: [], terminals: [{ name: "terminal-3f9a1c" }] });
    socket.receive({ type: "exit", session: "terminal-3f9a1c", code: 3 });
    socket.receive({ type: "exited", session: "terminal-3f9a1c", code: 130 });
    expect(ofType(events, "terminal_exit")).toEqual([
      { type: "terminal_exit", id: "terminal-3f9a1c", code: 3 },
      { type: "terminal_exit", id: "terminal-3f9a1c", code: 130 },
    ]);
    host.close();
  });

  it("leaves a seat's exit alone, since the seat's tab has its own", () => {
    const { host, events, socket } = connect(["terminals"]);
    socket.receive({ type: "exit", session: "frontend-eng-imp-dragonfly", code: 1 });
    expect(ofType(events, "terminal_exit")).toEqual([]);
    host.close();
  });
});

describe("a refused spawn", () => {
  it("says a remote device waits on the gating, in words, and not as a notice", () => {
    const { host, events, socket } = connect(["terminals"]);
    host.terminals.open("seat");
    socket.receive({ type: "error", id: socket.lastSent().id, code: 6, error: "refused", reason: "remote_terminal" });
    expect(ofType(events, "terminal_refused")).toEqual([{ type: "terminal_refused", label: "seat", text: terminalRefusal("remote_terminal", "refused") }]);
    expect(ofType(events, "notice")).toEqual([]);
    expect(terminalRefusal("remote_terminal", undefined)).toMatch(/host's own browser/);
    host.close();
  });

  it("turns a typing-guard refusal into read-only and ends the spawn with the guard's sentence", () => {
    const { host, events, socket } = connect(["terminals"]);
    host.terminals.open("seat");
    socket.receive({ type: "error", id: socket.lastSent().id, code: 6, error: "refused", reason: "peer_unread" });
    expect(ofType(events, "typing").at(-1)).toEqual({ type: "typing", typing: { allowed: false, reason: "peer_unread" } });
    expect(ofType(events, "terminal_refused")).toHaveLength(1);
    host.close();
  });

  it("carries the daemon's own words for any other refusal, or a plain fallback", () => {
    expect(terminalRefusal(undefined, "cwd is not a directory")).toBe("cwd is not a directory");
    expect(terminalRefusal(undefined, undefined)).toBe("The host would not open a terminal.");
  });

  it("does not turn an unrelated error into a refusal", () => {
    const { host, events, socket } = connect(["terminals"]);
    host.terminals.open("seat");
    socket.receive({ type: "error", id: "someone-else", code: 3, error: "the ask is gone" });
    expect(ofType(events, "terminal_refused")).toEqual([]);
    expect(ofType(events, "notice")).toEqual([{ type: "notice", text: "the ask is gone" }]);
    host.close();
  });
});
