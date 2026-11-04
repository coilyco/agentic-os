import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RETRY_SPAN_MS } from "./backoff";
import type { HostEvent } from "./protocol";
import { DaemonHost } from "./daemon-host";

// The link rules of COI-3352 against a socket the test controls: a link that goes silent,
// a return to the page, and an hour of tries that ends.
class FakeSocket {
  static all: FakeSocket[] = [];
  sent: { type: string }[] = [];
  closed = false;
  private handlers = new Map<string, ((event: unknown) => void)[]>();
  constructor() {
    FakeSocket.all.push(this);
  }
  addEventListener(type: string, handler: (event: unknown) => void): void {
    this.handlers.set(type, [...(this.handlers.get(type) ?? []), handler]);
  }
  send(line: string): void {
    this.sent.push(JSON.parse(line));
  }
  close(): void {
    this.closed = true;
  }
  fire(type: string, event: unknown = {}): void {
    for (const handler of this.handlers.get(type) ?? []) handler(event);
  }
  welcome(): void {
    this.fire("open");
    this.fire("message", { data: JSON.stringify({ type: "welcome", format: "aterm.daemon.v1", version: "1" }) });
  }
  sessions(): void {
    this.fire("message", { data: JSON.stringify({ type: "sessions", sessions: [] }) });
  }
}

const listeners = new Map<string, (() => void)[]>();
const target = {
  addEventListener: (type: string, handler: () => void) => listeners.set(type, [...(listeners.get(type) ?? []), handler]),
  removeEventListener: (type: string, handler: () => void) => listeners.set(type, (listeners.get(type) ?? []).filter((each) => each !== handler)),
};
const doc = { ...target, visibilityState: "visible" };
const wake = (type: "focus" | "online" | "visibilitychange") => {
  for (const handler of listeners.get(type) ?? []) handler();
};
const probes = (socket: FakeSocket) => socket.sent.filter((frame) => frame.type === "list").length;
const last = () => FakeSocket.all.at(-1)!;

let host: DaemonHost;
let events: HostEvent[];

/** A host that answered once, so a loss is a restart to ride out. */
function answered(): FakeSocket {
  host = new DaemonHost("ws://test");
  events = [];
  host.subscribe((event) => events.push(event));
  const first = last();
  first.welcome();
  first.sessions();
  return first;
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(Math, "random").mockReturnValue(0.5);
  FakeSocket.all = [];
  listeners.clear();
  doc.visibilityState = "visible";
  vi.stubGlobal("WebSocket", FakeSocket);
  vi.stubGlobal("window", target);
  vi.stubGlobal("document", doc);
});

afterEach(() => {
  host?.close();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("a link that carries nothing back", () => {
  it("is declared lost after a probe goes unanswered, though the socket never closes", () => {
    const first = answered();
    vi.advanceTimersByTime(20_000);
    expect(probes(first)).toBe(1);
    vi.advanceTimersByTime(7_000);
    expect(FakeSocket.all).toHaveLength(1);
    vi.advanceTimersByTime(1_500);
    expect(events.at(-1)).toMatchObject({ type: "reconnecting", attempt: 1 });
    vi.advanceTimersByTime(1000);
    expect(FakeSocket.all).toHaveLength(2);
    expect(first.closed).toBe(true);
  });

  it("is kept when any frame comes back before the probe's deadline", () => {
    const first = answered();
    // A daemon that answers each probe, here for five minutes.
    for (let n = 0; n < 15; n++) {
      vi.advanceTimersByTime(20_000);
      first.sessions();
    }
    expect(probes(first)).toBe(15);
    expect(FakeSocket.all).toHaveLength(1);
  });

  it("is tested the moment the person returns to the page", () => {
    const first = answered();
    wake("visibilitychange");
    expect(probes(first)).toBe(1);
    vi.advanceTimersByTime(9_000);
    expect(events.at(-1)).toMatchObject({ type: "reconnecting" });
  });

  it("counts a late close from the dropped socket once", () => {
    const first = answered();
    wake("focus");
    vi.advanceTimersByTime(9_000);
    first.fire("close");
    vi.advanceTimersByTime(1_500);
    expect(FakeSocket.all).toHaveLength(2);
    expect(events.filter((event) => event.type === "reconnecting" && event.retryAt !== null)).toHaveLength(1);
  });
});

describe("coming back to the page", () => {
  it("dials at once instead of waiting out the backoff, from focus, online or visibility", () => {
    for (const type of ["focus", "online", "visibilitychange"] as const) {
      FakeSocket.all = [];
      listeners.clear();
      const first = answered();
      first.fire("close");
      vi.advanceTimersByTime(500);
      expect(FakeSocket.all).toHaveLength(1);
      wake(type);
      expect(FakeSocket.all, type).toHaveLength(2);
      host.close();
    }
  });

  it("does nothing while the page is hidden", () => {
    const first = answered();
    first.fire("close");
    doc.visibilityState = "hidden";
    wake("visibilitychange");
    expect(FakeSocket.all).toHaveLength(1);
  });

  it("replaces a dial that has hung, and spares one that only just left", () => {
    answered().fire("close");
    vi.advanceTimersByTime(1000);
    const hung = last();
    expect(FakeSocket.all).toHaveLength(2);
    wake("focus");
    expect(FakeSocket.all).toHaveLength(2);
    vi.advanceTimersByTime(2_000);
    wake("focus");
    expect(FakeSocket.all).toHaveLength(3);
    expect(hung.closed).toBe(true);
    hung.fire("close");
    expect(events.filter((event) => event.type === "reconnecting" && event.retryAt !== null)).toHaveLength(1);
  });

  it("starts the hour over, since the short waits are the useful ones", () => {
    const first = answered();
    first.fire("close");
    for (let n = 0; n < 5; n++) {
      vi.advanceTimersByTime(40_000);
      last().fire("close");
    }
    expect(events.at(-1)).toMatchObject({ type: "reconnecting", attempt: 6 });
    wake("focus");
    last().fire("close");
    expect(events.at(-1)).toMatchObject({ type: "reconnecting", attempt: 1 });
  });
});

describe("the hour of tries", () => {
  it("makes its last automatic dial about an hour after the loss, then rests", () => {
    answered().fire("close");
    let elapsed = 0;
    // A refused dial closes at once, so each try is a new socket that closes.
    while (!events.some((event) => event.type === "resting") && elapsed < RETRY_SPAN_MS * 2) {
      vi.advanceTimersByTime(1000);
      elapsed += 1000;
      const fresh = last();
      if (!fresh.closed && fresh !== FakeSocket.all[0]) fresh.fire("close");
    }
    expect(events.some((event) => event.type === "resting")).toBe(true);
    // Twenty tries, the last failing, then the rest.
    expect(FakeSocket.all).toHaveLength(21);
    expect(elapsed).toBeGreaterThanOrEqual(RETRY_SPAN_MS * 0.99);
    expect(elapsed).toBeLessThanOrEqual(RETRY_SPAN_MS * 1.01 + 15_000);
  });

  it("dials again on the next return, and on Retry, with nothing scheduled in between", () => {
    answered().fire("close");
    for (let n = 0; n < 20; n++) {
      vi.advanceTimersByTime(RETRY_SPAN_MS);
      last().fire("close");
    }
    expect(events.at(-1)).toEqual({ type: "resting" });
    const count = FakeSocket.all.length;
    vi.advanceTimersByTime(RETRY_SPAN_MS * 3);
    expect(FakeSocket.all).toHaveLength(count);
    wake("online");
    expect(FakeSocket.all).toHaveLength(count + 1);
    last().fire("close");
    expect(events.at(-1)).toMatchObject({ type: "reconnecting", attempt: 1 });
  });
});
