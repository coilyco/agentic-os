import { afterEach, describe, expect, it, vi } from "vitest";
import { BrokerClient } from "./broker";
import { DaemonHost } from "./daemon-host";
import { foundKinds, newFoundHosts, parseFound } from "./discovery";
import { parseMachineKind } from "./machine-kind";
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
const kinds = (events: HostEvent[]) => events.flatMap((event) => (event.type === "machine_kind" ? [event.kind] : []));

function connect(welcome: Record<string, unknown>) {
  vi.stubGlobal("WebSocket", StubSocket);
  const host = new DaemonHost("ws://stub");
  const events: HostEvent[] = [];
  host.subscribe((event) => events.push(event));
  const socket = StubSocket.last;
  socket.receive({ ...WELCOME, ...welcome });
  return { host, events, socket };
}

afterEach(() => vi.unstubAllGlobals());

describe("parseMachineKind", () => {
  it("knows the three kinds and nothing else", () => {
    expect(["personal", "ephemeral", "durable"].map(parseMachineKind)).toEqual(["personal", "ephemeral", "durable"]);
    expect([undefined, "", "Durable", "server", 3].map(parseMachineKind)).toEqual([undefined, undefined, undefined, undefined, undefined]);
  });
});

describe("the attached host's kind", () => {
  it("reads the kind from a welcome that lists host-kind, and replays it to a later subscriber", () => {
    const { host, events } = connect({ features: ["host-kind"], host_kind: "durable" });
    expect(kinds(events)).toEqual(["durable"]);
    expect(host.machineKind).toBeDefined();
    const late: HostEvent[] = [];
    host.subscribe((event) => late.push(event));
    expect(kinds(late)).toEqual(["durable"]);
    host.close();
  });

  it("offers no picker for a daemon that predates the feature, even if a kind rides along", () => {
    const { host, events } = connect({ features: ["terminals"], host_kind: "durable" });
    expect(kinds(events)).toEqual([null]);
    expect(host.machineKind).toBeUndefined();
    host.close();
  });

  it("follows a change pushed with no id, such as `aterm host kind` on the machine", () => {
    const { host, events, socket } = connect({ features: ["host-kind"], host_kind: "personal" });
    socket.receive({ type: "host_kind", host_kind: "durable" });
    socket.receive({ type: "host_kind", host_kind: "nonsense" });
    expect(kinds(events)).toEqual(["personal", "durable"]);
    host.close();
  });

  it("sends the set frame and resolves the kind the daemon now holds", async () => {
    const { host, events, socket } = connect({ features: ["host-kind"], host_kind: "personal" });
    const done = host.machineKind!.set("ephemeral");
    const asked = sent(socket).at(-1)!;
    expect(asked).toMatchObject({ type: "host_kind", host_kind: "ephemeral" });
    socket.receive({ type: "host_kind", id: asked.id, host_kind: "ephemeral" });
    await expect(done).resolves.toBe("ephemeral");
    expect(kinds(events).at(-1)).toBe("ephemeral");
    host.close();
  });

  it("refuses a locked device with the typing guard's lock, and keeps the kind shown", async () => {
    const { host, events, socket } = connect({ features: ["host-kind", "passkey"], host_kind: "personal", typing: { allowed: false, reason: "passkey_required" } });
    const done = host.machineKind!.set("durable");
    const asked = sent(socket).at(-1)!;
    socket.receive({ type: "error", id: asked.id, code: 6, error: "input refused", reason: "passkey_required" });
    const refusal = await done.catch((error: unknown) => error);
    expect(refusal).toBeInstanceOf(Refusal);
    expect((refusal as Refusal).reason).toBe("passkey_required");
    expect(events.filter((event) => event.type === "typing").at(-1)).toMatchObject({ typing: { allowed: false, reason: "passkey_required" } });
    expect(kinds(events)).toEqual(["personal"]);
    host.close();
  });
});

describe("the kind in the lists", () => {
  it("carries each tailnet peer's kind, and none for a peer that sent none or an unknown one", () => {
    const found = parseFound([
      { name: "server", host: "server.example.ts.net", port: "7419", host_kind: "durable" },
      { name: "laptop", host: "laptop.example.ts.net", port: "7419" },
      { name: "odd", host: "odd.example.ts.net", port: "7419", host_kind: "mainframe" },
    ]);
    expect(found.map((each) => each.machineKind)).toEqual(["durable", undefined, undefined]);
    expect([...foundKinds(found).values()]).toEqual(["durable"]);
    expect(newFoundHosts([], found).map((each) => each.machineKind)).toEqual(["durable", undefined, undefined]);
  });

  it("reads the broker's kind for each daemon, offline ones included", async () => {
    const body = { daemons: [{ daemon_id: "d1", name: "server", substrate: "tailnet", state: "offline", channels: 0, kind: "durable" }, { daemon_id: "d2", name: "old", substrate: "tailnet", state: "online", channels: 1 }] };
    const broker = new BrokerClient({ baseUrl: "https://broker.example", accessToken: async () => "t", fetch: async () => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } }) });
    const daemons = await broker.daemons();
    expect(daemons.map((each) => each.machineKind)).toEqual(["durable", undefined]);
  });
});
