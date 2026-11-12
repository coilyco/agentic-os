import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AccountsError } from "./accounts";
import { BrokerError } from "./broker";
import { type Brokered, DaemonHost } from "./daemon-host";
import type { HostEvent } from "./protocol";

// A brokered channel as the daemon speaks it: the welcome carries `unlock` and starts
// locked, and one `unlock` frame lifts the lock (references/treehouse-broker.md).
class StubSocket {
  static all: StubSocket[] = [];
  readonly url: string;
  sent: string[] = [];
  closed = false;
  private handlers = new Map<string, ((event: unknown) => void)[]>();
  constructor(url: string) {
    this.url = url;
    StubSocket.all.push(this);
  }
  addEventListener(type: string, handler: (event: unknown) => void): void {
    this.handlers.set(type, [...(this.handlers.get(type) ?? []), handler]);
  }
  send(line: string): void {
    this.sent.push(line);
  }
  // A real socket reports its own close.
  close(): void {
    this.closed = true;
    this.drop();
  }
  receive(frame: Record<string, unknown>): void {
    for (const handler of this.handlers.get("message") ?? []) handler({ data: JSON.stringify(frame) });
  }
  drop(): void {
    for (const handler of this.handlers.get("close") ?? []) handler({});
  }
  frames(): Record<string, unknown>[] {
    return this.sent.map((line) => JSON.parse(line) as Record<string, unknown>);
  }
}

const DAEMON = "dmn_01HZX3KA4B8N";
const WELCOME = {
  type: "welcome",
  format: "aterm.daemon.v1",
  features: ["unlock-token", "terminals"],
  unlock: { daemon_id: DAEMON, nonce: "n3Qk9mV2xT7pLw0aZc5RyA" },
  typing: { allowed: false, reason: "passkey_required" },
};
const settle = () => vi.advanceTimersByTimeAsync(0);

function brokered(options: Partial<Brokered> = {}) {
  const tickets = vi.fn<Brokered["dial"]>().mockImplementation(async () => `wss://broker.example/v1/channel/t_${StubSocket.all.length}`);
  const unlock = vi.fn<Brokered["unlock"]>().mockResolvedValue("unlock.token.one");
  const via: Brokered = { dial: tickets, unlock, ...options };
  const host = new DaemonHost("ignored", via);
  const events: HostEvent[] = [];
  host.subscribe((event) => events.push(event));
  return { host, events, tickets, unlock };
}
const typings = (events: HostEvent[]) => events.filter((event) => event.type === "typing").map((event) => (event as { typing: unknown }).typing);
const lastSocket = () => StubSocket.all.at(-1)!;

beforeEach(() => {
  vi.useFakeTimers();
  StubSocket.all = [];
  vi.stubGlobal("WebSocket", StubSocket);
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("dialing through the broker", () => {
  it("opens the ticket URL it was handed, and says hello as on any daemon", async () => {
    const { host } = brokered();
    await settle();
    expect(lastSocket().url).toBe("wss://broker.example/v1/channel/t_0");
    host.close();
  });

  it("asks for a new ticket on every dial, because a ticket opens once", async () => {
    const { host, tickets } = brokered();
    await settle();
    lastSocket().receive(WELCOME);
    lastSocket().drop();
    await vi.advanceTimersByTimeAsync(2000);
    expect(tickets).toHaveBeenCalledTimes(2);
    expect(StubSocket.all.map((socket) => socket.url)).toEqual(["wss://broker.example/v1/channel/t_0", "wss://broker.example/v1/channel/t_1"]);
    host.close();
  });

  it("opens nothing when it is closed while the ticket is still being fetched", async () => {
    let release: (url: string) => void = () => {};
    const { host } = brokered({ dial: () => new Promise((resolve) => (release = resolve)) });
    host.close();
    release("wss://broker.example/v1/channel/t_late");
    await settle();
    expect(StubSocket.all).toEqual([]);
  });

  it("tells a person the machine is offline when it never answered, and does not retry in the dark", async () => {
    const { events, tickets } = brokered({ dial: vi.fn().mockRejectedValue(new BrokerError("daemon_offline", "the daemon is not connected", 503, 5)) });
    await settle();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(events).toContainEqual({ type: "closed", reason: "This machine is offline. It is not connected to the broker." });
    expect(tickets).not.toHaveBeenCalled();
    expect(StubSocket.all).toEqual([]);
  });

  it("rides out an offline machine once it has answered, with the usual reconnecting state", async () => {
    const dial = vi.fn<Brokered["dial"]>().mockResolvedValueOnce("wss://broker.example/v1/channel/t_a").mockRejectedValueOnce(new BrokerError("daemon_offline", "x", 503)).mockResolvedValue("wss://broker.example/v1/channel/t_b");
    const { host, events } = brokered({ dial });
    await settle();
    lastSocket().receive(WELCOME);
    lastSocket().drop();
    await vi.advanceTimersByTimeAsync(1500);
    expect(events.some((event) => event.type === "reconnecting")).toBe(true);
    expect(events.some((event) => event.type === "closed")).toBe(false);
    await vi.advanceTimersByTimeAsync(5000);
    expect(StubSocket.all.map((socket) => socket.url)).toContain("wss://broker.example/v1/channel/t_b");
    host.close();
  });

  it("stops for good when accounts or the broker says the session is over", async () => {
    const dial = vi.fn<Brokered["dial"]>().mockResolvedValueOnce("wss://broker.example/v1/channel/t_a").mockRejectedValue(new AccountsError("unauthorized", "Sign in first.", 401));
    const { events } = brokered({ dial });
    await settle();
    lastSocket().receive(WELCOME);
    lastSocket().drop();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(events).toContainEqual({ type: "closed", reason: "Your sign-in ended. Sign in again to reconnect." });
    expect(dial).toHaveBeenCalledTimes(2);
  });
});

describe("unlocking a brokered channel", () => {
  it("shows the channel locked first, and sends the unlock token in the unlock frame, never as token", async () => {
    const { host, events, unlock } = brokered();
    await settle();
    lastSocket().receive(WELCOME);
    expect(typings(events)).toEqual([{ allowed: false, reason: "passkey_required" }]);
    await settle();
    expect(unlock).toHaveBeenCalledWith(DAEMON, "n3Qk9mV2xT7pLw0aZc5RyA");
    const frame = lastSocket().frames().find((each) => each.type === "unlock")!;
    expect(frame).toMatchObject({ type: "unlock", unlock_token: "unlock.token.one" });
    expect(frame).not.toHaveProperty("token");
    expect(typeof frame.id).toBe("string");
    host.close();
  });

  it("types only after the daemon answers typing allowed", async () => {
    const { host, events } = brokered();
    await settle();
    lastSocket().receive(WELCOME);
    await settle();
    const id = lastSocket().frames().find((each) => each.type === "unlock")!.id;
    expect(typings(events).at(-1)).toEqual({ allowed: false, reason: "passkey_required" });
    lastSocket().receive({ type: "typing", id, typing: { allowed: true } });
    expect(typings(events).at(-1)).toEqual({ allowed: true });
    host.close();
  });

  it("leaves the channel locked when the daemon refuses the token, and does not send the same token again", async () => {
    const { host, events, unlock } = brokered();
    await settle();
    lastSocket().receive(WELCOME);
    await settle();
    const id = lastSocket().frames().find((each) => each.type === "unlock")!.id;
    lastSocket().receive({ type: "error", id, reason: "unlock_refused", error: "this unlock was refused, reconnect to try again" });
    expect(typings(events).at(-1)).toEqual({ allowed: false, reason: "unlock_refused" });
    await vi.advanceTimersByTimeAsync(5000);
    expect(unlock).toHaveBeenCalledTimes(1);
    expect(lastSocket().frames().filter((each) => each.type === "unlock")).toHaveLength(1);
    host.close();
  });

  it("keeps typing refused as passkey_required for a bad or reused token, with typed input still refused", async () => {
    const { host, events } = brokered();
    await settle();
    lastSocket().receive(WELCOME);
    await settle();
    host.input("session-1", "ls\n");
    const inputId = lastSocket().frames().find((each) => each.type === "input")!.id;
    lastSocket().receive({ type: "error", id: inputId, code: 6, error: "input refused", reason: "passkey_required" });
    expect(typings(events).at(-1)).toEqual({ allowed: false, reason: "passkey_required" });
    host.close();
  });

  it("asks for a fresh token on the same channel when unlocking again, up to a few refusals", async () => {
    const unlock = vi.fn<Brokered["unlock"]>().mockResolvedValueOnce("first").mockResolvedValueOnce("second");
    const { host } = brokered({ unlock });
    await settle();
    lastSocket().receive(WELCOME);
    await settle();
    lastSocket().receive({ type: "error", id: lastSocket().frames().find((each) => each.type === "unlock")!.id, reason: "unlock_refused" });
    host.unlockAgain();
    await settle();
    const tokens = lastSocket().frames().filter((each) => each.type === "unlock").map((each) => each.unlock_token);
    expect(tokens).toEqual(["first", "second"]);
    host.close();
  });

  it("redials for a new nonce after three refusals, since the daemon spends the channel's nonce after five", async () => {
    const { host, tickets } = brokered();
    await settle();
    lastSocket().receive(WELCOME);
    for (let round = 0; round < 3; round++) {
      await settle();
      lastSocket().receive({ type: "error", id: lastSocket().frames().filter((each) => each.type === "unlock").at(-1)!.id, reason: "unlock_refused" });
      host.unlockAgain();
    }
    await vi.advanceTimersByTimeAsync(2000);
    expect(tickets.mock.calls.length).toBeGreaterThanOrEqual(2);
    host.close();
  });

  it("says accounts did not unlock it, and stays locked, when the token cannot be had", async () => {
    const { host, events } = brokered({ unlock: vi.fn().mockRejectedValue(new AccountsError("network", "Accounts could not be reached.", 0)) });
    await settle();
    lastSocket().receive(WELCOME);
    await settle();
    expect(typings(events).at(-1)).toEqual({ allowed: false, reason: "unlock_failed" });
    expect(events).toContainEqual({ type: "notice", text: "Accounts did not unlock this connection. Accounts could not be reached." });
    expect(lastSocket().frames().some((each) => each.type === "unlock")).toBe(false);
    host.close();
  });

  it("sends no unlock for a welcome that carries no unlock member", async () => {
    const { host, unlock } = brokered();
    await settle();
    lastSocket().receive({ ...WELCOME, unlock: undefined });
    await settle();
    expect(unlock).not.toHaveBeenCalled();
    host.close();
  });

  it("leaves a direct connection alone even if a welcome carries an unlock member", async () => {
    const host = new DaemonHost("ws://stub");
    await settle();
    lastSocket().receive(WELCOME);
    await settle();
    expect(lastSocket().frames().some((each) => each.type === "unlock")).toBe(false);
    host.close();
  });
});
