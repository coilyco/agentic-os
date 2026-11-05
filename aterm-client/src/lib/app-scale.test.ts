import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// COI-3363: a row's work must not grow with the roster. The sidebar asks about every
// row each frame, so one slow answer is N of them. The limits have 100 times of room.
class NoSocket {
  addEventListener(): void {}
  send(): void {}
  close(): void {}
}

async function app() {
  vi.stubGlobal("WebSocket", NoSocket);
  vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  return import("./app.svelte");
}

beforeEach(() => {
  // The clock stays real, since the test times a pass.
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval"] });
  vi.resetModules();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const SEATS = 3000;
const roster = (count: number) =>
  Array.from({ length: count }, (_, at) => ({ id: `seat-${at}`, role: "frontend-eng", seat: "claude", identity: `Seat-${at}`, state: "idle", pending: 0, drafting: false, paste: true, degraded: [] }));

/** Milliseconds a pass over every row takes, as the sidebar makes one. */
function pass(askLineFor: (id: string) => string | null, count: number): number {
  const started = process.hrtime.bigint();
  for (let at = 0; at < count; at++) askLineFor(`seat-${at}`);
  return Number(process.hrtime.bigint() - started) / 1e6;
}

describe("a pass over every row of a large roster", () => {
  it("costs almost nothing while no ask is open", { timeout: 60_000 }, async () => {
    const mod = await app();
    mod.app.sessions = roster(SEATS) as never;
    const ms = pass(mod.askLineFor, SEATS);
    expect(ms, `${SEATS} rows took ${ms.toFixed(0)} ms`).toBeLessThan(100);
  });

  it("builds the live seat set once, not once per row, while an ask is open", { timeout: 60_000 }, async () => {
    const mod = await app();
    mod.app.sessions = roster(SEATS) as never;
    mod.app.asks = { a1: { id: "a1", session: "seat-7", header: "", question: "Which?", options: [], allowOther: false, multi: false } } as never;
    const ms = pass(mod.askLineFor, SEATS);
    expect(ms, `${SEATS} rows took ${ms.toFixed(0)} ms`).toBeLessThan(300);
    expect(mod.askLineFor("seat-7")).not.toBeNull();
    expect(mod.askLineFor("seat-8")).toBeNull();
  });
});
