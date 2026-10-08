import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// A daemon whose socket the test can close. app.svelte.ts dials on import,
// so the browser globals it reads are stubbed first.
class FakeDaemon {
  static all: FakeDaemon[] = [];
  sent: string[] = [];
  private handlers = new Map<string, ((event: unknown) => void)[]>();
  constructor() {
    FakeDaemon.all.push(this);
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
  drop(): void {
    for (const handler of this.handlers.get("close") ?? []) handler({});
  }
}

const SEAT = { name: "s1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0 };
const ASK = { id: "k1", session: "s1", question: "Which?", options: [{ label: "One" }] };
const welcome = (version: string) => ({ type: "welcome", format: "aterm.daemon.v1", version });

async function attached(version = "0.443.0") {
  vi.stubGlobal("WebSocket", FakeDaemon);
  vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  const mod = await import("./app.svelte");
  const host = mod.app.hosts.find((each) => each.kind === "daemon")!;
  host.status = { kind: "online", sessionCount: 1 };
  mod.selectHost(host);
  const daemon = FakeDaemon.all.at(-1)!;
  daemon.receive(welcome(version));
  daemon.receive({ type: "sessions", sessions: [SEAT] });
  daemon.receive({ type: "ask", ask: ASK });
  mod.selectSession("s1");
  return { ...mod, host, daemon };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(Math, "random").mockReturnValue(0.5);
  vi.resetModules();
  FakeDaemon.all = [];
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("the daemon restarts under an open window", () => {
  it("keeps the open seat, the roster and the same connection while it redials", async () => {
    const { app, host, daemon } = await attached();
    const connection = app.connection;
    daemon.drop();
    expect(app.link).toMatchObject({ state: "reconnecting", attempt: 1 });
    expect(app.selectedSession).toBe("s1");
    expect(app.sessions.map((session) => session.id)).toEqual(["s1"]);
    expect(app.attachedHostId).toBe(host.id);
    expect(host.status.kind).toBe("online");
    expect(app.connection).toBe(connection);
  });

  it("reconnects with no click, and says it is back for a few seconds", async () => {
    const { app, daemon } = await attached();
    daemon.drop();
    vi.advanceTimersByTime(1000);
    const again = FakeDaemon.all.at(-1)!;
    expect(again).not.toBe(daemon);
    again.receive(welcome("0.443.0"));
    again.receive({ type: "sessions", sessions: [SEAT] });
    expect(app.link.state).toBe("restored");
    expect(app.selectedSession).toBe("s1");
    expect(app.newerBuild).toBe(false);
    vi.advanceTimersByTime(5000);
    expect(app.link.state).toBe("live");
  });

  it("drops the asks of the daemon that died, and takes the ones the new daemon still holds", async () => {
    const { app, daemon } = await attached();
    expect(Object.keys(app.asks)).toEqual(["k1"]);
    daemon.drop();
    expect(Object.keys(app.asks)).toEqual(["k1"]);
    vi.advanceTimersByTime(1000);
    const again = FakeDaemon.all.at(-1)!;
    again.receive(welcome("0.443.0"));
    expect(app.asks).toEqual({});
    again.receive({ type: "ask", ask: { ...ASK, id: "k2" } });
    expect(Object.keys(app.asks)).toEqual(["k2"]);
  });

  it("offers a reload when the daemon that answered is a newer build", async () => {
    const { app, daemon } = await attached("0.443.0");
    daemon.drop();
    vi.advanceTimersByTime(1000);
    FakeDaemon.all.at(-1)!.receive(welcome("0.444.0"));
    expect(app.newerBuild).toBe(true);
  });

  it("lets Retry skip the wait", async () => {
    const { app, retryNow, daemon } = await attached();
    daemon.drop();
    const dialed = FakeDaemon.all.length;
    retryNow();
    expect(FakeDaemon.all).toHaveLength(dialed + 1);
    expect(app.link).toMatchObject({ state: "reconnecting", retryAt: null });
  });

  it("keeps the old not-answering state for a host that never answered the page", async () => {
    vi.stubGlobal("WebSocket", FakeDaemon);
    vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
    vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
    const { app, selectHost } = await import("./app.svelte");
    const host = app.hosts.find((each) => each.kind === "daemon")!;
    host.status = { kind: "online", sessionCount: 1 };
    selectHost(host);
    FakeDaemon.all.at(-1)!.drop();
    expect(host.status.kind).toBe("unreachable");
    expect(app.link.state).toBe("live");
    expect(app.attachedHostId).toBeNull();
  });
});
