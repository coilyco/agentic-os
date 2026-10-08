import { afterEach, describe, expect, it, vi } from "vitest";
import type { HostEvent } from "./protocol";
import { DaemonHost, daemonUrl, hostLabel, launchRefusal, toAsk, toMessage, toSession, toView } from "./daemon-host";

describe("aterm.daemon.v1 mapping", () => {
  it("reads a ready session as idle and carries the draft flag", () => {
    const session = toSession({ name: "session-48c6d1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "zsh", ready: true, bracketed_paste: true, kai_drafting: true, pending: 2 });
    expect(session).toEqual({ id: "session-48c6d1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "zsh", state: "idle", pending: 2, drafting: true, paste: true, degraded: [] });
  });

  it("carries the startup steps a session launched without", () => {
    const view = { name: "s", role: "r", identity: "i", seat: "codex", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0, degraded: ["card", "telemetry"] };
    expect(toSession(view).degraded).toEqual(["card", "telemetry"]);
  });

  it("carries a context reading, and drops an absent window instead of showing zero", () => {
    const base = { name: "s", role: "r", identity: "i", seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0 };
    expect(toSession({ ...base, context: { tokens: 1050, source: "claude" } }).context).toEqual({ tokens: 1050, source: "claude" });
    expect(toSession({ ...base, context: { tokens: 5000, window: 258400, source: "codex" } }).context).toEqual({ tokens: 5000, window: 258400, source: "codex" });
    expect("context" in toSession(base)).toBe(false);
  });

  it("reads a session not at its prompt as working", () => {
    expect(toSession({ name: "s", role: "r", identity: "i", seat: "codex", ready: false, bracketed_paste: false, kai_drafting: false, pending: 0 }).state).toBe("working");
  });

  it("splits the sender and keeps the failure reason", () => {
    const message = toMessage({ id: "a1", from: "eng-platform Beetle-Ox", target: "game-dev", state: "failed", reason: "no live session" });
    expect(message).toEqual({ id: "a1", from: { role: "eng-platform", identity: "Beetle-Ox" }, target: "game-dev", session: null, state: "failed", reason: "no live session" });
  });
});

describe("launchRefusal", () => {
  it("names a refused slug or seat apart from a launch that failed on the host", () => {
    expect(launchRefusal(2, "launch needs a role slug")).toMatch(/would not launch/);
    expect(launchRefusal(5, "exit status 4")).toMatch(/failed on the host: exit status 4/);
  });
});

describe("toAsk", () => {
  it("fills the optional ask_choice fields", () => {
    expect(toAsk({ id: "a", session: "s", question: "Which?", options: [{ label: "One" }] })).toEqual({
      id: "a", session: "s", header: "", question: "Which?", options: [{ label: "One", description: "" }], allowOther: false, multi: false,
    });
  });
});

describe("daemonUrl", () => {
  it("dials the local daemon from the dev server", () => {
    expect(daemonUrl({ protocol: "http:", host: "localhost:5173" }, true)).toBe("ws://127.0.0.1:7419");
  });

  it("dials back to the origin that served a build, secure when the page is", () => {
    expect(daemonUrl({ protocol: "https:", host: "mac.example.ts.net" }, false)).toBe("wss://mac.example.ts.net/");
    expect(daemonUrl({ protocol: "http:", host: "127.0.0.1:7419" }, false)).toBe("ws://127.0.0.1:7419/");
  });

  it("lets an explicit address win", () => {
    expect(daemonUrl({ protocol: "https:", host: "x" }, false, "ws://127.0.0.1:7420")).toBe("ws://127.0.0.1:7420");
  });
});

describe("hostLabel", () => {
  it("names the machine from the tower, and says this Mac locally", () => {
    expect(hostLabel({ hostname: "kais-macbook-pro.example.ts.net" }, false)).toBe("kais-macbook-pro");
    expect(hostLabel({ hostname: "localhost" }, false)).toBe("this Mac");
  });
});

// Only what DaemonHost touches: a frame in becomes `message`, a frame out is recorded.
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
  drop(): void {
    for (const handler of this.handlers.get("close") ?? []) handler({});
  }
}

function connect(): { host: DaemonHost; events: HostEvent[]; socket: StubSocket } {
  vi.stubGlobal("WebSocket", StubSocket);
  const host = new DaemonHost("ws://stub");
  const events: HostEvent[] = [];
  host.subscribe((event) => events.push(event));
  return { host, events, socket: StubSocket.last };
}

const typings = (events: HostEvent[]) => events.filter((event) => event.type === "typing");

describe("the typing guard", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("reads read-only from the welcome, before anything is typed", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", typing: { allowed: false, reason: "session_descendant" } });
    expect(typings(events)).toEqual([{ type: "typing", typing: { allowed: false, reason: "session_descendant" } }]);
    host.close();
  });

  it("stays quiet when a daemon predates the guard, and replays the state to a later subscriber", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1" });
    expect(typings(events)).toEqual([]);
    socket.receive({ type: "error", id: "c9", code: 6, error: "input refused", reason: "peer_unread" });
    const late: HostEvent[] = [];
    host.subscribe((event) => late.push(event));
    expect(typings(late)).toEqual([{ type: "typing", typing: { allowed: false, reason: "peer_unread" } }]);
    host.close();
  });

  it("turns a refused input into read-only without a notice, and an unrelated error into a notice only", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", typing: { allowed: true } });
    socket.receive({ type: "error", id: "c5", code: 6, error: "input refused", reason: "session_descendant" });
    socket.receive({ type: "error", id: "c6", code: 3, error: "the ask is gone", reason: "settled" });
    expect(typings(events).at(-1)).toEqual({ type: "typing", typing: { allowed: false, reason: "session_descendant" } });
    expect(events.filter((event) => event.type === "notice")).toEqual([{ type: "notice", text: "the ask is gone" }]);
    host.close();
  });

  it("fails a launch the guard refused with the reason, not as a launch that broke on the host", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1" });
    host.launch("frontend-eng", "claude");
    const id = JSON.parse(socket.sent.at(-1) ?? "{}").id;
    socket.receive({ type: "error", id, code: 6, error: "launch refused", reason: "session_descendant" });
    const failed = events.filter((event) => event.type === "launch" && event.state === "failed");
    expect(failed).toHaveLength(1);
    expect(failed[0]).toMatchObject({ role: "frontend-eng", text: expect.stringMatching(/started by an agent session/) });
    host.close();
  });
});

const browsers = (events: HostEvent[]) => events.flatMap((event) => (event.type === "browser" ? [event.browser] : []));

describe("the shared browser", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("offers no browser channel until the welcome lists the feature", () => {
    const { host, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", features: ["send-new"] });
    expect(host.browser).toBeUndefined();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", features: ["browser"] });
    expect(host.browser).toBeDefined();
    host.close();
  });

  it("sends the frames the daemon reads, with the pane size when it is given", () => {
    const { host, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", features: ["browser"] });
    const sent = () => JSON.parse(socket.sent.at(-1) ?? "{}");
    host.browser?.watch("s", { width: 390, height: 600 });
    expect(sent()).toMatchObject({ type: "browser_watch", session: "s", width: 390, height: 600 });
    host.browser?.control("s", true, true);
    expect(sent()).toMatchObject({ type: "browser_control", session: "s", take: true, force: true });
    host.browser?.input("s", "mouse", { type: "mousePressed", x: 1, y: 2 });
    expect(sent()).toMatchObject({ type: "browser_input", session: "s", kind: "mouse", params: { type: "mousePressed", x: 1, y: 2 } });
    host.browser?.navigate("s", "https://example.com/");
    expect(sent()).toMatchObject({ type: "browser_navigate", session: "s", url: "https://example.com/" });
    host.browser?.unwatch("s");
    expect(sent()).toMatchObject({ type: "browser_unwatch", session: "s" });
    host.close();
  });

  it("reads who holds the browser against the name the daemon gave this connection", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", features: ["browser"] });
    socket.receive({ type: "browser_state", session: "s", state: "live", driver: "person", holder: "screen-a", client: "screen-a", url: "https://example.com/", title: "Example" });
    socket.receive({ type: "browser_state", session: "s", state: "live", driver: "person", holder: "screen-b", client: "screen-a" });
    socket.receive({ type: "browser_state", session: "s", state: "closed", driver: "agent", client: "screen-a", reason: "The browser process exited." });
    const [mine, theirs, closed] = browsers(events);
    expect(mine).toMatchObject({ state: "live", driver: "person", heldHere: true, url: "https://example.com/", title: "Example" });
    expect(theirs).toMatchObject({ heldHere: false, holder: "screen-b" });
    expect(closed).toMatchObject({ state: "closed", driver: "agent", heldHere: false, reason: "The browser process exited." });
    host.close();
  });

  it("draws a frame onto its browser and keeps it through the next state", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", features: ["browser"] });
    socket.receive({ type: "browser_frame", session: "s", data: "AAAA", seq: 1, metadata: { device_width: 1280, device_height: 800, offset_top: 0, page_scale_factor: 1 } });
    expect(browsers(events)).toEqual([]);
    socket.receive({ type: "browser_state", session: "s", state: "live", driver: "agent", client: "c" });
    socket.receive({ type: "browser_frame", session: "s", data: "AAAA", seq: 7, metadata: { device_width: 1280, device_height: 800, offset_top: 4, page_scale_factor: 2 } });
    socket.receive({ type: "browser_state", session: "s", state: "live", driver: "person", holder: "c", client: "c" });
    const [, framed, later] = browsers(events);
    expect(framed?.frame).toMatchObject({ src: "data:image/jpeg;base64,AAAA", seq: 7, metadata: { deviceWidth: 1280, deviceHeight: 800, offsetTop: 4, pageScaleFactor: 2 } });
    expect(later?.frame?.seq).toBe(7);
    expect(later?.heldHere).toBe(true);
    host.close();
  });

  it("shows a browser refusal as a notice, not as read-only typing", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", features: ["browser"], typing: { allowed: true } });
    socket.receive({ type: "error", id: "c3", code: 1, error: "another screen has control, take it over with force", reason: "browser_held" });
    expect(events.filter((event) => event.type === "notice")).toEqual([{ type: "notice", text: "another screen has control, take it over with force" }]);
    expect(typings(events).at(-1)).toEqual({ type: "typing", typing: { allowed: true } });
    host.close();
  });
});

const view = (name: string, ready: boolean) => ({ name, role: "eng-platform", identity: "Beetle-Ox", seat: "claude", ready, bracketed_paste: true, kai_drafting: false, pending: 0 });
const framesOf = (socket: StubSocket) => socket.sent.map((line) => JSON.parse(line) as { type: string; session?: string; replay?: boolean });

describe("another seat launching beside an open terminal", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("leaves the open seat's attachment alone and flags the new one as starting", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1" });
    socket.receive({ type: "sessions", sessions: [view("eng-platform-beetle-ox", true)] });
    host.attach("eng-platform-beetle-ox", 30, 100);
    const before = framesOf(socket).length;

    // The daemon pushes the roster again for each seat the launch path opens.
    for (const count of [2, 3, 4]) {
      const launching = Array.from({ length: count - 1 }, (_, at) => view(`eng-platform-beetle-ox-${at + 2}`, false));
      socket.receive({ type: "sessions", sessions: [view("eng-platform-beetle-ox", true), ...launching] });
    }

    const after = framesOf(socket).slice(before);
    expect(after.filter((frame) => frame.session === "eng-platform-beetle-ox")).toEqual([]);
    // The monitor watches each new seat without replay, so nothing on screen resets.
    expect(after.every((frame) => frame.type === "attach" && frame.replay === false)).toBe(true);
    const last = events.filter((event) => event.type === "sessions").at(-1);
    expect(last?.type === "sessions" && last.sessions.map((session) => [session.id, session.starting ?? false])).toEqual([
      ["eng-platform-beetle-ox", false],
      ["eng-platform-beetle-ox-2", true],
      ["eng-platform-beetle-ox-3", true],
      ["eng-platform-beetle-ox-4", true],
    ]);
    host.close();
  });

  it("reads a seat that is ready as no longer starting", () => {
    expect(toSession(view("s", true)).starting).toBeUndefined();
    expect(toSession(view("s", false)).starting).toBe(true);
  });
});

const WELCOME = { type: "welcome", format: "aterm.daemon.v1" };
const sentFrames = (socket: StubSocket) => socket.sent.map((line) => JSON.parse(line) as Record<string, unknown>);
const VIEW = {
  id: "v1",
  server: "sysadmin",
  tool: "disk",
  resource_uri: "ui://sysadmin/disk",
  html: "<p>74.6%</p>",
  csp: { connect_domains: ["https://stats.example"], resource_domains: [] },
  prefers_border: true,
  tool_input: { host: "kai-server" },
};

describe("toView", () => {
  it("maps the frame to the client's shape and drops the fields the daemon left out", () => {
    expect(toView("s1", VIEW)).toEqual({
      id: "v1", session: "s1", server: "sysadmin", tool: "disk", resourceUri: "ui://sysadmin/disk", html: "<p>74.6%</p>",
      csp: { connectDomains: ["https://stats.example"], resourceDomains: [] }, prefersBorder: true, toolInput: { host: "kai-server" },
    });
    const bare = toView("s1", { id: "v2", server: "x", tool: "t", resource_uri: "ui://x", html: "" });
    expect(bare).toEqual({ id: "v2", session: "s1", server: "x", tool: "t", resourceUri: "ui://x", html: "", toolInput: {} });
    expect("csp" in bare || "prefersBorder" in bare).toBe(false);
  });
});

describe("MCP Apps views", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("subscribes to views only when the welcome lists mcp-apps, and offers the channel only then", () => {
    const without = connect();
    without.socket.receive(WELCOME);
    expect(sentFrames(without.socket).some((frame) => frame.channel === "views")).toBe(false);
    expect(without.host.views).toBeUndefined();
    without.host.close();

    const withFeature = connect();
    withFeature.socket.receive({ ...WELCOME, features: ["mcp-apps"] });
    expect(sentFrames(withFeature.socket).filter((frame) => frame.type === "subscribe").map((frame) => frame.channel)).toEqual(["sessions", "views"]);
    expect(withFeature.host.views).toBeDefined();
    withFeature.host.close();
  });

  it("turns view, view_update and view_closed into the events the panel already reads", () => {
    const { host, events, socket } = connect();
    socket.receive({ ...WELCOME, features: ["mcp-apps"] });
    socket.receive({ type: "view", session: "s1", view: VIEW });
    socket.receive({ type: "view_update", session: "s1", view_id: "v1", tool_result: { content: [{ type: "text", text: "74.6" }] } });
    socket.receive({ type: "view_update", session: "s1", view_id: "v2", cancelled: "the call failed" });
    socket.receive({ type: "view_closed", view_id: "v1" });
    const seen = events.filter((event) => event.type.startsWith("view"));
    expect(seen.map((event) => event.type)).toEqual(["view", "view_update", "view_update", "view_closed"]);
    expect(seen[0]).toMatchObject({ view: { id: "v1", session: "s1", resourceUri: "ui://sysadmin/disk" } });
    expect(seen[1]).toEqual({ type: "view_update", id: "v1", toolResult: { content: [{ type: "text", text: "74.6" }] } });
    expect(seen[2]).toEqual({ type: "view_update", id: "v2", cancelled: "the call failed" });
    expect(seen[3]).toEqual({ type: "view_closed", id: "v1" });
    host.close();
  });

  it("resolves a view call on its view_result and ignores a result for a call it did not make", async () => {
    const { host, socket } = connect();
    socket.receive({ ...WELCOME, features: ["mcp-apps"] });
    const pending = host.views!.call("v1", "tools/call", { name: "disk", arguments: {} });
    const call = sentFrames(socket).at(-1)!;
    expect(call).toMatchObject({ type: "view_call", view_id: "v1", method: "tools/call", params: { name: "disk", arguments: {} } });
    socket.receive({ type: "view_result", id: "c999", view_id: "v1", result: "stranger" });
    socket.receive({ type: "view_result", id: call.id, view_id: "v1", result: { content: [] } });
    await expect(pending).resolves.toEqual({ content: [] });
    host.close();
  });

  it("rejects a view call on an error with its id, in the daemon's words, without a stray notice", async () => {
    const { host, events, socket } = connect();
    socket.receive({ ...WELCOME, features: ["mcp-apps"] });
    const pending = host.views!.call("v1", "tools/call", { name: "hidden" });
    socket.receive({ type: "error", id: sentFrames(socket).at(-1)!.id, code: 2, error: "tool is not visible to the app" });
    await expect(pending).rejects.toThrow("tool is not visible to the app");
    expect(events.filter((event) => event.type === "notice")).toEqual([]);
    host.close();
  });

  it("treats a view call the typing guard refused as a refusal: the read-only state, and the reason as the error", async () => {
    const { host, events, socket } = connect();
    socket.receive({ ...WELCOME, features: ["mcp-apps"], typing: { allowed: true } });
    const pending = host.views!.call("v1", "tools/call", { name: "disk" });
    socket.receive({ type: "error", id: sentFrames(socket).at(-1)!.id, code: 6, error: "view_call refused", reason: "session_descendant" });
    await expect(pending).rejects.toThrow(/started by an agent session/);
    expect(events.filter((event) => event.type === "typing").at(-1)).toEqual({ type: "typing", typing: { allowed: false, reason: "session_descendant" } });
    host.close();
  });

  it("sends view_close with the view id, and rejects calls still pending when the socket drops", async () => {
    const { host, socket } = connect();
    socket.receive({ ...WELCOME, features: ["mcp-apps"] });
    host.views!.close("v1");
    expect(sentFrames(socket).at(-1)).toMatchObject({ type: "view_close", view_id: "v1" });
    const pending = host.views!.call("v1", "resources/read", { uri: "ui://x" });
    socket.drop();
    await expect(pending).rejects.toThrow("The daemon closed the connection.");
    host.close();
  });
});
