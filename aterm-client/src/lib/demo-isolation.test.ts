import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Each test re-imports the app state module, which a busy machine takes over 5 s to do.
vi.setConfig({ testTimeout: 30_000 });

// COI-3315: the demo never opens a connection to a real host. Every way this page can
// reach out is replaced by a recorder, with a real saved host on the device to tempt it.
const REAL = "kai-server.example-tailnet.ts.net";

const reached: string[] = [];
const writes: string[] = [];
const store = new Map<string, string>([["aterm.hosts.v1", JSON.stringify([{ label: "Real host", address: REAL }])]]);

class RecordingSocket {
  constructor(url: string) {
    reached.push(`WebSocket ${url}`);
  }
  addEventListener(): void {}
  removeEventListener(): void {}
  send(): void {}
  close(): void {}
}

class RecordingXhr {
  open(method: string, url: string): void {
    reached.push(`XMLHttpRequest ${method} ${url}`);
  }
  send(): void {}
}

class RecordingEventSource {
  constructor(url: string) {
    reached.push(`EventSource ${url}`);
  }
  close(): void {}
}

async function page() {
  vi.stubGlobal("WebSocket", RecordingSocket);
  vi.stubGlobal("XMLHttpRequest", RecordingXhr);
  vi.stubGlobal("EventSource", RecordingEventSource);
  vi.stubGlobal("fetch", async (url: unknown) => {
    reached.push(`fetch ${String(url)}`);
    throw new TypeError("offline");
  });
  vi.stubGlobal("navigator", { sendBeacon: (url: string) => (reached.push(`sendBeacon ${url}`), true) });
  vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => (writes.push(key), void store.set(key, value)),
    removeItem: (key: string) => void store.delete(key),
  });
  return await import("./app.svelte");
}

/** The saved host on the device, answering, so picking it attaches. */
function realHost(mod: Awaited<ReturnType<typeof page>>) {
  const real = mod.app.hosts.find((host) => host.address.includes(REAL))!;
  real.status = { kind: "online", sessionCount: 1 };
  return real;
}

beforeEach(() => {
  reached.length = 0;
  writes.length = 0;
  vi.useFakeTimers();
  vi.resetModules();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("the demo opens no connection to a real host", () => {
  it("sees a socket when a real host is picked, so the recorder is not blind", async () => {
    const mod = await page();
    expect(reached.some((each) => each.includes(REAL))).toBe(true);
    reached.length = 0;
    mod.selectHost(realHost(mod));
    expect(reached.some((each) => each.startsWith("WebSocket") && each.includes(REAL))).toBe(true);
  });

  it("reaches nothing from the first tap through every seat, the question, the terminal, a launch, and ten idle minutes", async () => {
    const mod = await page();
    // The page dialled its saved host as it loaded: the connect screen, before the tap.
    expect(reached.length).toBeGreaterThan(0);
    // It answers, so discovery has a host to ask each minute unless the demo stops it.
    realHost(mod);
    reached.length = 0;

    mod.enterDemo();
    expect(mod.inDemo()).toBe(true);
    expect(mod.app.selectedSession).toBe(mod.app.sessions[0]!.id);

    vi.advanceTimersByTime(2000);
    expect(Object.keys(mod.app.asks)).toHaveLength(1);
    expect(mod.app.messages).toHaveLength(2);

    for (const session of mod.app.sessions) {
      mod.selectSession(session.id);
      mod.app.connection!.attach(session.id, 24, 80);
      mod.app.connection!.input(session.id, "hello\r");
      vi.advanceTimersByTime(1500);
    }

    const ask = Object.values(mod.app.asks)[0]!;
    mod.app.connection!.answer(ask.id, [0]);

    mod.openTerminal(mod.app.sessions[0]!.id);
    vi.advanceTimersByTime(400);
    const shell = mod.app.terminals[0]!;
    mod.app.connection!.attach(shell.id, 24, 80);
    for (const line of ["help\r", "ls\r", "curl http://example.com\r", "ssh kai@kai-server\r"]) mod.app.connection!.input(shell.id, line);

    mod.app.connection!.launch("game-dev", "claude");
    mod.closeTerminal(mod.app.sessions[0]!.id);

    // Past the 60 s discovery timer ten times over, with the tab visible.
    vi.advanceTimersByTime(10 * 60_000);

    expect(reached).toEqual([]);
  });

  it("writes nothing to the device about hosts or terminals, and leaves with nothing of the demo", async () => {
    const mod = await page();
    const before = JSON.stringify([...store]);
    writes.length = 0;

    mod.enterDemo();
    vi.advanceTimersByTime(2000);
    mod.openTerminal(mod.app.sessions[0]!.id);
    vi.advanceTimersByTime(400);
    mod.closeTerminal(mod.app.sessions[0]!.id);
    mod.leaveDemo();

    expect(writes).toEqual([]);
    expect(JSON.stringify([...store])).toBe(before);
    expect(mod.inDemo()).toBe(false);
    expect(mod.app.connection).toBeNull();
    expect(mod.app.attachedHostId).toBeNull();
    expect(mod.app.selectedHostId).toBeNull();
    expect(mod.app.sessions).toEqual([]);
    expect(mod.app.messages).toEqual([]);
    expect(mod.app.asks).toEqual({});
    expect(mod.app.terminals).toEqual([]);
    expect(mod.app.terminalClosed.filter((id) => id.startsWith("demo-"))).toEqual([]);
    expect(Object.keys(mod.app.terminalSeats).filter((id) => id.startsWith("demo-"))).toEqual([]);
  });

  it("goes back to the connect screen, where the demo can be entered again", async () => {
    const mod = await page();
    mod.enterDemo();
    mod.leaveDemo();
    mod.enterDemo();
    expect(mod.inDemo()).toBe(true);
    expect(mod.app.sessions.length).toBe(4);
  });

  it("is ended by picking a real host, which then dials only that host", async () => {
    const mod = await page();
    mod.enterDemo();
    reached.length = 0;
    mod.selectHost(realHost(mod));
    expect(mod.inDemo()).toBe(false);
    expect(reached.every((each) => each.includes(REAL))).toBe(true);
    expect(mod.app.sessions.some((session) => session.id.startsWith("demo-"))).toBe(false);
  });
});
