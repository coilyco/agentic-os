import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// COI-3352: a launch whose remembered host is down must select it, or the recheck that
// sees the daemon return has nothing to attach. The bed clicks the host first.
class Dial {
  static all: Dial[] = [];
  private handlers = new Map<string, ((event: unknown) => void)[]>();
  constructor() {
    Dial.all.push(this);
  }
  addEventListener(type: string, handler: (event: unknown) => void): void {
    this.handlers.set(type, [...(this.handlers.get(type) ?? []), handler]);
  }
  send(): void {}
  close(): void {}
  private emit(type: string, event: unknown): void {
    for (const handler of this.handlers.get(type) ?? []) handler(event);
  }
  refuse(): void {
    this.emit("error", {});
    this.emit("close", {});
  }
  answer(): void {
    this.emit("message", { data: JSON.stringify({ type: "welcome", format: "aterm.daemon.v1", version: "0.540.0" }) });
    this.emit("message", { data: JSON.stringify({ type: "sessions", sessions: [] }) });
  }
}

const LAST_OPEN = '{"host":"local"}';

async function launched() {
  vi.stubGlobal("WebSocket", Dial);
  vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  vi.stubGlobal("localStorage", { getItem: (key: string) => (key === "aterm.last-open.v1" ? LAST_OPEN : null), setItem() {} });
  vi.stubGlobal("fetch", () => Promise.reject(new Error("offline")));
  return import("./app.svelte");
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(Math, "random").mockReturnValue(0.5);
  vi.resetModules();
  Dial.all = [];
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("a launch while the remembered host is down", () => {
  it("selects the host, and attaches on its own once the daemon answers", async () => {
    const { app } = await launched();
    const host = app.hosts.find((each) => each.id === "local")!;
    // The launch probe finds nothing.
    Dial.all.at(-1)!.refuse();
    await vi.advanceTimersByTimeAsync(0);
    expect(host.status.kind).toBe("unreachable");
    expect(app.selectedHostId, "the host is on screen, with its Retry").toBe("local");
    expect(app.attachedHostId).toBeNull();
    // The daemon comes back. The next recheck probe is answered with no tap.
    await vi.advanceTimersByTimeAsync(5000);
    Dial.all.at(-1)!.answer();
    await vi.advanceTimersByTimeAsync(0);
    expect(host.status.kind).toBe("online");
    expect(app.attachedHostId, "the host is attached").toBe("local");
  });
});
