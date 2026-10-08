import { afterEach, describe, expect, it, vi } from "vitest";
import { DaemonHost } from "./daemon-host";
import type { HostEvent } from "./protocol";
import { Refusal } from "./typing";

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

const WELCOME = { type: "welcome", format: "aterm.daemon.v1" };
const sent = (socket: StubSocket) => socket.sent.map((line) => JSON.parse(line) as Record<string, unknown>);

function connect(features?: string[]) {
  vi.stubGlobal("WebSocket", StubSocket);
  const host = new DaemonHost("ws://stub");
  const events: HostEvent[] = [];
  host.subscribe((event) => events.push(event));
  const socket = StubSocket.last;
  socket.receive({ ...WELCOME, ...(features ? { features } : {}) });
  return { host, events, socket };
}

afterEach(() => vi.unstubAllGlobals());

describe("Web Push frames", () => {
  it("offers the channel only when the welcome lists web-push, and says so in the features event", () => {
    const none = connect(["terminals"]);
    expect(none.host.push).toBeUndefined();
    expect(none.events.filter((event) => event.type === "features")).toEqual([{ type: "features", terminals: true }]);
    none.host.close();

    const some = connect(["web-push"]);
    expect(some.host.push).toBeDefined();
    expect(some.events.filter((event) => event.type === "features")).toEqual([{ type: "features", terminals: false, webPush: true }]);
    some.host.close();
  });

  it("asks push_key and resolves with the key the daemon answers", async () => {
    const { host, socket } = connect(["web-push"]);
    const key = host.push!.key();
    const asked = sent(socket).at(-1)!;
    expect(asked.type).toBe("push_key");
    socket.receive({ type: "push_key", id: asked.id, key: "BKEY" });
    await expect(key).resolves.toBe("BKEY");
    host.close();
  });

  it("rejects a push_key answer with no key", async () => {
    const { host, socket } = connect(["web-push"]);
    const key = host.push!.key();
    socket.receive({ type: "push_key", id: sent(socket).at(-1)!.id });
    await expect(key).rejects.toThrow(/no push key/);
    host.close();
  });

  it("sends the subscription as it is and resolves on push_subscribed", async () => {
    const { host, socket } = connect(["web-push"]);
    const subscription = { endpoint: "https://fcm.googleapis.com/fcm/send/abc", keys: { p256dh: "p", auth: "a" } };
    const done = host.push!.subscribe(subscription);
    const asked = sent(socket).at(-1)!;
    expect(asked).toMatchObject({ type: "push_subscribe", push: subscription });
    socket.receive({ type: "push_subscribed", id: asked.id });
    await expect(done).resolves.toBeUndefined();
    host.close();
  });

  it("sends only the endpoint to unsubscribe and resolves on push_unsubscribed", async () => {
    const { host, socket } = connect(["web-push"]);
    const done = host.push!.unsubscribe("https://fcm.googleapis.com/fcm/send/abc");
    const asked = sent(socket).at(-1)!;
    expect(asked).toMatchObject({ type: "push_unsubscribe", push: { endpoint: "https://fcm.googleapis.com/fcm/send/abc" } });
    socket.receive({ type: "push_unsubscribed", id: asked.id });
    await expect(done).resolves.toBeUndefined();
    host.close();
  });

  it("rejects a locked device's subscribe as a Refusal that names passkey_required", async () => {
    const { host, socket } = connect(["web-push"]);
    const done = host.push!.subscribe({ endpoint: "https://fcm.googleapis.com/x" });
    socket.receive({ type: "error", id: sent(socket).at(-1)!.id, code: 6, error: "subscribe refused", reason: "passkey_required" });
    const error = await done.catch((each: unknown) => each);
    expect(error).toBeInstanceOf(Refusal);
    expect((error as Refusal).reason).toBe("passkey_required");
    host.close();
  });

  it("carries the daemon's own words when it refuses for another reason", async () => {
    const { host, socket } = connect(["web-push"]);
    const done = host.push!.subscribe({ endpoint: "http://example.com/" });
    socket.receive({ type: "error", id: sent(socket).at(-1)!.id, code: 2, error: "push endpoint must be https" });
    const error = (await done.catch((each: unknown) => each)) as Refusal;
    expect(error.message).toBe("push endpoint must be https");
    expect(error.reason).toBeUndefined();
    host.close();
  });
});
