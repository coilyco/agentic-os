import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// app.svelte.ts dials hosts on import, so the browser globals it reads are stubbed first.
class NoSocket {
  addEventListener(): void {}
  send(): void {}
  close(): void {}
}

async function demo(search: string) {
  vi.stubGlobal("WebSocket", NoSocket);
  vi.stubGlobal("location", { search, hostname: "localhost", protocol: "http:", host: "localhost" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  const mod = await import("./app.svelte");
  const host = mod.app.hosts.find((each) => each.kind === "demo")!;
  mod.selectHost(host);
  return mod;
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetModules();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const SEAT = "sysadmin-senior-turtle-ox-gj84";

describe("opening a terminal beside a seat", () => {
  it("opens one shell when the daemon's reply comes after its list", async () => {
    const { app, openTerminal } = await demo("?terminals=ok");
    openTerminal(SEAT);
    openTerminal(SEAT);
    vi.advanceTimersByTime(400);
    expect(app.terminals.map((entry) => entry.label)).toEqual([SEAT]);
    expect(app.terminalOpening[SEAT]).toBeUndefined();
  });

  it("keeps opening, not ready, when the reply comes before the list, so the pane does not open a second", async () => {
    const { app, openTerminal } = await demo("?terminals=early");
    openTerminal(SEAT);
    vi.advanceTimersByTime(260);
    expect(app.terminals).toEqual([]);
    expect(app.terminalOpening[SEAT]).toBe(true);
    vi.advanceTimersByTime(200);
    expect(app.terminals.map((entry) => entry.label)).toEqual([SEAT]);
    expect(app.terminalOpening[SEAT]).toBeUndefined();
  });

  it("remembers which seat a shell sits beside, so a reload finds it", async () => {
    const { app, openTerminal } = await demo("?terminals=early");
    openTerminal(SEAT);
    vi.advanceTimersByTime(400);
    expect(Object.values(app.terminalSeats)).toEqual([SEAT]);
  });

  it("turns a refusal into words and stops opening", async () => {
    const { app, openTerminal } = await demo("?terminals=refuse");
    openTerminal(SEAT);
    vi.advanceTimersByTime(300);
    expect(app.terminalRefusals[SEAT]).toBeTruthy();
    expect(app.terminalOpening[SEAT]).toBeUndefined();
  });

  it("says the host did not answer when a spawn never does", async () => {
    const { app, openTerminal } = await demo("?terminals=silent");
    openTerminal(SEAT);
    vi.advanceTimersByTime(8100);
    expect(app.terminalRefusals[SEAT]).toMatch(/did not answer/);
    expect(app.terminalOpening[SEAT]).toBeUndefined();
  });

  it("does not open on a host whose welcome lacks the feature", async () => {
    const { app, openTerminal } = await demo("?terminals=off");
    openTerminal(SEAT);
    vi.advanceTimersByTime(400);
    expect(app.terminalSupport).toBe("no");
    expect(app.terminalOpening[SEAT]).toBeUndefined();
    expect(app.terminals).toEqual([]);
  });
});

describe("a shell ending", () => {
  async function opened() {
    const mod = await demo("?terminals=ok");
    mod.openTerminal(SEAT);
    vi.advanceTimersByTime(400);
    const id = mod.app.terminals[0]!.id;
    mod.app.connection!.attach(id, 24, 80);
    return { ...mod, id };
  }

  it("stays on screen as ended, with its code, when it exits", async () => {
    const { app, id } = await opened();
    app.connection!.input(id, "exit 4\r");
    expect(app.terminals).toEqual([{ id, label: SEAT, exit: { code: 4 } }]);
  });

  it("does not come back on its own after Kai closes it", async () => {
    const { app, closeTerminal } = await opened();
    closeTerminal(SEAT);
    vi.advanceTimersByTime(400);
    expect(app.terminals).toEqual([]);
    expect(app.terminalClosed).toContain(SEAT);
  });
});
