import { describe, expect, it } from "vitest";
import { checkHops, type HopId, type HopReport, LIMITS, reportText, type SocketIo } from "./hops";
import type { Io } from "./reach";

const ADDRESS = "wss://mac.example.ts.net:7419/";

class FakeSocket {
  private listeners = new Map<string, Array<(event: unknown) => void>>();
  sent: string[] = [];
  addEventListener(type: string, listener: (event: unknown) => void): void {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }
  send(data: string): void {
    this.sent.push(data);
  }
  close(): void {}
  emit(type: string, event: unknown = {}): void {
    for (const listener of this.listeners.get(type) ?? []) listener(event);
  }
}

type Step = Response | Error;

function seams(options: { asks?: Step[]; socket?: (socket: FakeSocket) => void; protocol?: string }) {
  const asks = [...(options.asks ?? [])];
  const calls: string[] = [];
  const reach: Io = {
    ask: async (url, init) => {
      calls.push(`${init.mode ?? "cors"} ${url}`);
      const step = asks.shift() ?? new TypeError("Failed to fetch");
      if (step instanceof Error) throw step;
      return step;
    },
    timeoutMs: 50,
    now: () => 0,
  };
  const socketIo: SocketIo = {
    open: () => {
      const socket = new FakeSocket();
      setTimeout(() => options.socket?.(socket), 0);
      return socket as unknown as WebSocket;
    },
    timeoutMs: 50,
    now: () => 0,
  };
  return { reach, socket: socketIo, here: { protocol: options.protocol ?? "https:", hostname: "page.example" }, calls };
}

const admitted = () => new Response('{"layer":"ok"}', { status: 200 });
const refusal = (layer: string, text: string) => new Response(`aterm daemon: ${text}\n`, { status: 403, headers: { "X-Aterm-Refusal": layer } });
const welcomes = (socket: FakeSocket) => {
  socket.emit("open");
  socket.emit("message", { data: JSON.stringify({ type: "welcome", format: "aterm.daemon.v1" }) });
};

const status = (report: HopReport) => Object.fromEntries(report.hops.map((hop) => [hop.id, hop.status])) as Record<HopId, string>;

describe("checkHops", () => {
  it("passes every hop for an admitted device whose socket greets it", async () => {
    const s = seams({ asks: [admitted()], socket: welcomes });
    const report = await checkHops(ADDRESS, s);
    expect(Object.values(status(report))).toEqual(Array(6).fill("pass"));
    expect(report.verdict).toBe("Every hop passed just now.");
    expect(report.hops[3]!.basis).toBe("seen");
    expect(report.hops[0]!.basis).toBe("inferred");
  });

  it("names the admission hop when the daemon refuses the device or the address", async () => {
    for (const layer of ["ownership", "name"]) {
      const report = await checkHops(ADDRESS, seams({ asks: [refusal(layer, "that device is not admitted")] }));
      expect(status(report)).toMatchObject({ resolve: "pass", route: "pass", tls: "pass", http: "pass", socket: "skipped", admit: "fail" });
      expect(report.verdict).toBe("Broke at hop 6, Daemon admits this device.");
      expect(report.hops[5]!.observed).toContain("that device is not admitted");
    }
  });

  it("names the upgrade hop when the daemon refuses this page's origin", async () => {
    const report = await checkHops(ADDRESS, seams({ asks: [refusal("origin", "this page may not open a session socket")] }));
    expect(status(report)).toMatchObject({ http: "pass", socket: "fail", admit: "skipped" });
    expect(report.verdict).toBe("Broke at hop 5, Websocket upgrade.");
  });

  it("says a failed request could be any of the first three hops, and names none", async () => {
    const report = await checkHops(ADDRESS, seams({ asks: [new TypeError("Failed to fetch"), new TypeError("Failed to fetch")] }));
    expect(status(report)).toMatchObject({ resolve: "maybe", route: "maybe", tls: "maybe", http: "skipped", socket: "skipped" });
    expect(report.hops.every((hop) => hop.status !== "fail")).toBe(true);
    expect(report.verdict).toBe("Broke at hop 1, 2 or 3. A page cannot tell which.");
  });

  it("reads a hang as the route, and does not ask a silent host twice", async () => {
    const s = seams({ asks: [new DOMException("aborted", "AbortError")] });
    const report = await checkHops(ADDRESS, s);
    expect(status(report)).toMatchObject({ route: "fail", http: "skipped" });
    expect(report.verdict).toBe("Most likely broke at hop 2, Tailnet route. A page cannot rule out hop 1 or 3.");
    expect(s.calls).toHaveLength(1);
  });

  it("names the upgrade hop when HTTP worked and the socket never opened", async () => {
    const report = await checkHops(ADDRESS, seams({ asks: [admitted()], socket: (socket) => socket.emit("close", { code: 1006, reason: "" }) }));
    expect(status(report)).toMatchObject({ http: "pass", socket: "fail", admit: "skipped" });
    expect(report.hops[4]!.observed).toContain("1006");
  });

  it("explains an unreadable answer followed by a failed upgrade as a refusal the page may not read", async () => {
    const report = await checkHops(ADDRESS, seams({ asks: [new TypeError("Failed to fetch"), new Response(null, { status: 200 })], socket: (socket) => socket.emit("close", { code: 1006, reason: "" }) }));
    expect(status(report)).toMatchObject({ resolve: "pass", route: "pass", tls: "pass", http: "pass", socket: "fail" });
    expect(report.hops[4]!.cause).toContain("origin");
  });

  it("reports the daemon's own words when its welcome carries an error", async () => {
    const report = await checkHops(ADDRESS, seams({
      asks: [admitted()],
      socket: (socket) => {
        socket.emit("open");
        socket.emit("message", { data: JSON.stringify({ type: "welcome", error: "unsupported format x" }) });
      },
    }));
    expect(status(report)).toMatchObject({ socket: "pass", admit: "fail" });
    expect(report.hops[5]!.observed).toContain("unsupported format x");
  });

  it("reports the close code and reason of a socket the daemon cut", async () => {
    const report = await checkHops(ADDRESS, seams({
      asks: [admitted()],
      socket: (socket) => {
        socket.emit("open");
        socket.emit("close", { code: 1008, reason: "policy" });
      },
    }));
    expect(status(report)).toMatchObject({ socket: "pass", admit: "fail" });
    expect(report.hops[5]!.observed).toContain("1008");
    expect(report.hops[5]!.observed).toContain("policy");
  });

  it("reports a socket that opened and never greeted", async () => {
    const report = await checkHops(ADDRESS, seams({ asks: [admitted()], socket: (socket) => socket.emit("open") }));
    expect(status(report).admit).toBe("fail");
    expect(report.hops[5]!.observed).toContain("never sent its welcome");
  });

  it("knows before asking that a secure page will not open a plain socket", async () => {
    const s = seams({ protocol: "https:" });
    const report = await checkHops("ws://mac.example.ts.net:7419/", s);
    expect(status(report)).toMatchObject({ tls: "fail", http: "skipped" });
    expect(report.hops[2]!.basis).toBe("seen");
    expect(s.calls).toHaveLength(0);
  });

  it("lets a secure page open a plain socket to the machine it runs on", async () => {
    const s = seams({ asks: [admitted()], socket: welcomes, protocol: "https:" });
    const report = await checkHops("ws://127.0.0.1:7419/", s);
    expect(report.verdict).toBe("Every hop passed just now.");
  });

  it("does not ask when the address is not a websocket address", async () => {
    const s = seams({});
    const report = await checkHops("nonsense", s);
    expect(status(report).resolve).toBe("fail");
    expect(s.calls).toHaveLength(0);
  });
});

describe("reportText", () => {
  it("is a message to prod-director carrying every hop, its basis, and what a page cannot see", async () => {
    const report = await checkHops(ADDRESS, seams({ asks: [new TypeError("Failed to fetch"), new TypeError("Failed to fetch")] }));
    const text = reportText(report, { label: "mac", page: "https://coilyco.dev/aterm/", agent: "Chrome/147", online: true, link: "cellular" });
    expect(text.startsWith("To prod-director:")).toBe(true);
    expect(text).toContain(ADDRESS);
    expect(text).toContain("Result: Broke at hop 1, 2 or 3. A page cannot tell which.");
    for (const [index, hop] of report.hops.entries()) expect(text).toContain(`${index + 1}. ${hop.title}`);
    for (const line of LIMITS) expect(text).toContain(line);
    expect(text).toContain("Link type: cellular");
  });

  it("keeps the house voice: no em-dashes, no semicolons", async () => {
    const report = await checkHops(ADDRESS, seams({ asks: [new TypeError("Failed to fetch"), new TypeError("Failed to fetch")] }));
    const text = reportText(report, { label: "mac", page: "p", agent: "a", online: null, link: "" });
    expect(text).not.toMatch(/[—;]/);
    for (const hop of report.hops) expect(hop.cause + hop.observed).not.toMatch(/[—;]/);
  });
});
