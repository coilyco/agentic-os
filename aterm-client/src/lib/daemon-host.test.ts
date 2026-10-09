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
  /** Every socket dialed, oldest first, to tell a redial from what it replaces. */
  static all: StubSocket[] = [];
  sent: string[] = [];
  private handlers = new Map<string, ((event: unknown) => void)[]>();
  constructor() {
    StubSocket.last = this;
    StubSocket.all.push(this);
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

  it("carries the followed tab and the open count, and drops them when the daemon sends none", () => {
    const { host, events, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1", features: ["browser"] });
    socket.receive({ type: "browser_state", session: "s", state: "live", driver: "agent", client: "c", tab: 2, tabs: 3, url: "https://b.example/", title: "B" });
    socket.receive({ type: "browser_state", session: "s", state: "live", driver: "agent", client: "c", url: "https://a.example/", title: "A" });
    const [many, bare] = browsers(events);
    expect(many).toMatchObject({ tab: 2, tabs: 3, title: "B" });
    expect(bare).not.toHaveProperty("tab");
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

describe("the passkey ceremony", () => {
  afterEach(() => vi.unstubAllGlobals());

  const LOCKED = { allowed: false, reason: "passkey_required" };
  const FEATURES = { ...WELCOME, features: ["typing-guard", "passkey"] };
  const OPTIONS = { publicKey: { rp: { id: "coilyco.dev", name: "aterm" }, user: { id: "AQID", name: "kai", displayName: "Kai" }, challenge: "AQ", rpId: "coilyco.dev" } };
  const credential = { id: "c", rawId: new Uint8Array([1]).buffer, type: "public-key", toJSON: () => ({ id: "made-by-browser" }) };

  function browser(create = vi.fn().mockResolvedValue(credential), get = vi.fn().mockResolvedValue(credential)) {
    vi.stubGlobal("navigator", { credentials: { create, get } });
    vi.stubGlobal("isSecureContext", true);
    vi.stubGlobal("location", { hostname: "coilyco.dev" });
    return { create, get };
  }
  const lastSent = (socket: StubSocket) => sentFrames(socket).at(-1)!;
  const typings = (events: HostEvent[]) => events.filter((event) => event.type === "typing");

  it("offers the channel only when the welcome lists passkey, and reads the standing off a locked welcome", () => {
    const none = connect();
    none.socket.receive(WELCOME);
    expect(none.host.passkey).toBeUndefined();
    none.host.close();

    const { host, events, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, passkey: "unenrolled" } });
    expect(host.passkey).toBeDefined();
    expect(typings(events)).toEqual([{ type: "typing", typing: { ...LOCKED, passkey: "unenrolled" } }]);
    host.close();
  });

  it("keeps the standing when a refusal frame arrives without it, and takes a typing push as the new state", () => {
    const { host, events, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, passkey: "enrolled" } });
    socket.receive({ type: "error", id: "c9", code: 6, error: "input refused", reason: "passkey_required" });
    expect(typings(events).at(-1)).toEqual({ type: "typing", typing: { ...LOCKED, passkey: "enrolled" } });
    socket.receive({ type: "typing", typing: { allowed: true } });
    expect(typings(events).at(-1)).toEqual({ type: "typing", typing: { allowed: true } });
    host.close();
  });

  it("enrolls: begin with the code, create in the browser, finish with the credential, resolve on passkey_enrolled", async () => {
    const { create } = browser();
    const { host, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, passkey: "unenrolled" } });
    const done = host.passkey!.enroll("ABCD-1234");
    const begin = lastSent(socket);
    expect(begin).toMatchObject({ type: "passkey_enroll_begin", enroll_code: "ABCD-1234" });
    socket.receive({ type: "passkey_enroll_options", id: begin.id, options: OPTIONS });
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("passkey_enroll_finish"));
    const finish = lastSent(socket);
    expect(finish.credential).toEqual({ id: "made-by-browser" });
    expect(create).toHaveBeenCalledOnce();
    socket.receive({ type: "passkey_enrolled", id: finish.id });
    await expect(done).resolves.toBeUndefined();
    host.close();
  });

  it("shows the daemon's own words for a wrong code, and does not claim the code is spent", async () => {
    browser();
    const { host, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, passkey: "unenrolled" } });
    const done = host.passkey!.enroll("WRONG");
    socket.receive({ type: "error", id: lastSent(socket).id, code: 2, error: "that is not the enrollment code" });
    await expect(done).rejects.toThrow(/^that is not the enrollment code$/);
    host.close();
  });

  it("says the code is used up when the browser ends the ceremony after the daemon took it", async () => {
    browser(vi.fn().mockRejectedValue(Object.assign(new Error("x"), { name: "NotAllowedError" })));
    const { host, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, passkey: "unenrolled" } });
    const done = host.passkey!.enroll("ABCD-1234");
    socket.receive({ type: "passkey_enroll_options", id: lastSent(socket).id, options: OPTIONS });
    await expect(done).rejects.toThrow(/closed or timed out.*That code is used up/);
    host.close();
  });

  it("sends no code at all when this browser cannot run a ceremony", async () => {
    vi.stubGlobal("navigator", {});
    const { host, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, passkey: "unenrolled" } });
    const before = socket.sent.length;
    await expect(host.passkey!.enroll("ABCD-1234")).rejects.toThrow(/cannot make or use passkeys/);
    expect(socket.sent.length).toBe(before);
    host.close();
  });

  it("asserts: begin, get in the browser, finish, resolve on passkey_asserted", async () => {
    const { get } = browser();
    const { host, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, passkey: "enrolled" } });
    const done = host.passkey!.assert();
    const begin = lastSent(socket);
    expect(begin.type).toBe("passkey_assert_begin");
    socket.receive({ type: "passkey_assert_options", id: begin.id, options: OPTIONS });
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("passkey_assert_finish"));
    const finish = lastSent(socket);
    expect(finish.credential).toEqual({ id: "made-by-browser" });
    expect(get).toHaveBeenCalledOnce();
    socket.receive({ type: "passkey_asserted", id: finish.id });
    await expect(done).resolves.toBeUndefined();
    host.close();
  });

  it("rejects a failed assertion in the daemon's words, so the person can begin again", async () => {
    browser();
    const { host, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, passkey: "enrolled" } });
    const done = host.passkey!.assert();
    socket.receive({ type: "passkey_assert_options", id: lastSent(socket).id, options: OPTIONS });
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("passkey_assert_finish"));
    socket.receive({ type: "error", id: lastSent(socket).id, code: 2, error: "the assertion did not verify" });
    await expect(done).rejects.toThrow("the assertion did not verify");
    host.close();
  });
});

// A daemon restart: holders keep every seat alive and the daemon is back in seconds.
describe("riding out a daemon restart", () => {
  const SEATS = {
    type: "sessions",
    sessions: [
      { name: "s1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0 },
      { name: "s2", role: "scientist", identity: "Frog-Ox", seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0 },
    ],
    terminals: [{ name: "t1" }],
  };
  const reconnecting = (events: HostEvent[]) => events.filter((event) => event.type === "reconnecting");
  const kinds = (socket: StubSocket) => sentFrames(socket).map((frame) => frame.type);

  function answering(version = "0.443.0") {
    vi.useFakeTimers();
    vi.spyOn(Math, "random").mockReturnValue(0.5);
    StubSocket.all = [];
    const dialed = connect();
    dialed.socket.receive({ ...WELCOME, version, features: ["terminals"] });
    dialed.socket.receive(SEATS);
    return dialed;
  }

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("redials with no click, says so, and never reports the host closed", () => {
    const { host, events, socket } = answering();
    socket.drop();
    expect(reconnecting(events)).toEqual([{ type: "reconnecting", attempt: 1, retryAt: Date.now() + 1000 }]);
    expect(StubSocket.all).toHaveLength(1);
    vi.advanceTimersByTime(1000);
    expect(StubSocket.all).toHaveLength(2);
    expect(reconnecting(events).at(-1)).toEqual({ type: "reconnecting", attempt: 1, retryAt: null });
    expect(events.some((event) => event.type === "closed")).toBe(false);
    host.close();
  });

  it("waits 1, 2, 4 seconds while the daemon stays down, then starts over once it answers", () => {
    const { host, events, socket } = answering();
    const start = Date.now();
    socket.drop();
    vi.advanceTimersByTime(1000);
    StubSocket.last.drop();
    vi.advanceTimersByTime(2000);
    StubSocket.last.drop();
    // Dials start at 1 s and 3 s, so the three waits end at 1 s, 3 s and 7 s.
    const due = reconnecting(events).flatMap((event) => (event.type === "reconnecting" && event.retryAt !== null ? [event.retryAt - start] : []));
    expect(due).toEqual([1000, 3000, 7000]);
    vi.advanceTimersByTime(4000);
    StubSocket.last.receive({ ...WELCOME, version: "0.443.0" });
    StubSocket.last.drop();
    expect(reconnecting(events).at(-1)).toMatchObject({ attempt: 1, retryAt: Date.now() + 1000 });
    host.close();
  });

  it("attaches the open seat again with replay at its last size, and only watches the rest", () => {
    const { host, events, socket } = answering();
    host.attach("s1", 30, 100);
    host.resize("s1", 40, 120);
    host.attach("t1", 20, 80);
    socket.drop();
    vi.advanceTimersByTime(1000);
    const again = StubSocket.last;
    again.receive({ ...WELCOME, version: "0.443.0" });
    expect(events.at(-1)).toEqual({ type: "features", terminals: false });
    expect(events.filter((event) => event.type === "reconnected")).toEqual([{ type: "reconnected", newerBuild: false }]);
    expect(kinds(again)).toEqual(["subscribe", "roster"]);
    again.receive(SEATS);
    const attaches = sentFrames(again).filter((frame) => frame.type === "attach");
    expect(attaches).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ session: "s1", replay: true, rows: 40, cols: 120 }),
        expect.objectContaining({ session: "t1", replay: true, rows: 20, cols: 80 }),
        expect.objectContaining({ session: "s2", replay: false }),
      ]),
    );
    expect(attaches).toHaveLength(3);
    host.close();
  });

  it("does not attach a seat the restarted daemon no longer lists", () => {
    const { host, socket } = answering();
    host.attach("s1", 30, 100);
    socket.drop();
    vi.advanceTimersByTime(1000);
    StubSocket.last.receive({ ...WELCOME, version: "0.443.0" });
    StubSocket.last.receive({ ...SEATS, sessions: [SEATS.sessions[1]], terminals: [] });
    const asked = sentFrames(StubSocket.last).filter((frame) => frame.type === "attach");
    expect(asked.map((frame) => frame.session)).toEqual(["s2"]);
    host.close();
  });

  it("drops what was typed into the dead link instead of landing it after the redial", () => {
    const { host, socket } = answering();
    socket.drop();
    host.input("s1", "rm -rf build\r");
    host.launch("frontend-eng", "claude");
    vi.advanceTimersByTime(1000);
    StubSocket.last.receive({ ...WELCOME, version: "0.443.0" });
    StubSocket.last.receive(SEATS);
    expect(kinds(StubSocket.last)).not.toContain("input");
    expect(kinds(StubSocket.last)).not.toContain("launch");
    expect(StubSocket.all[0]!.sent.some((line) => line.includes("rm -rf"))).toBe(false);
    host.close();
  });

  it("offers a reload only when the daemon that came back is a newer build", () => {
    const same = answering("0.443.0");
    same.socket.drop();
    vi.advanceTimersByTime(1000);
    StubSocket.last.receive({ ...WELCOME, version: "0.443.0" });
    expect(same.events.filter((event) => event.type === "reconnected")).toEqual([{ type: "reconnected", newerBuild: false }]);
    same.host.close();
    vi.restoreAllMocks();
    vi.useRealTimers();

    const upgraded = answering("0.443.0");
    upgraded.socket.drop();
    vi.advanceTimersByTime(1000);
    StubSocket.last.receive({ ...WELCOME, version: "0.444.0" });
    expect(upgraded.events.filter((event) => event.type === "reconnected")).toEqual([{ type: "reconnected", newerBuild: true }]);
    upgraded.host.close();
  });

  it("dials now on retry, and ignores a retry while a dial is already out", () => {
    const { host, events, socket } = answering();
    socket.drop();
    host.retry();
    expect(StubSocket.all).toHaveLength(2);
    expect(reconnecting(events).at(-1)).toMatchObject({ retryAt: null });
    host.retry();
    expect(StubSocket.all).toHaveLength(2);
    host.close();
  });

  it("gives up on a dial that never answers instead of waiting on it", () => {
    const { host, socket } = answering();
    socket.drop();
    vi.advanceTimersByTime(1000);
    const hung = StubSocket.last;
    const closed = vi.spyOn(hung, "close");
    vi.advanceTimersByTime(9999);
    expect(closed).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(closed).toHaveBeenCalledTimes(1);
    host.close();
  });

  it("stops for good when the daemon comes back speaking something else", () => {
    const { host, events, socket } = answering();
    socket.drop();
    vi.advanceTimersByTime(1000);
    StubSocket.last.receive({ type: "welcome", format: "aterm.daemon.v9" });
    expect(events.at(-1)).toEqual({ type: "closed", reason: "The daemon speaks aterm.daemon.v9." });
    StubSocket.last.drop();
    vi.advanceTimersByTime(120_000);
    expect(StubSocket.all).toHaveLength(2);
    host.close();
  });

  it("reports a host that never answered as closed, and does not redial it", () => {
    vi.useFakeTimers();
    StubSocket.all = [];
    const { host, events, socket } = connect();
    socket.drop();
    expect(events.at(-1)).toEqual({ type: "closed", reason: "The daemon closed the connection." });
    vi.advanceTimersByTime(120_000);
    expect(StubSocket.all).toHaveLength(1);
    host.close();
  });

  it("stops redialing when the page closes the connection", () => {
    const { host, socket } = answering();
    socket.drop();
    host.close();
    vi.advanceTimersByTime(120_000);
    expect(StubSocket.all).toHaveLength(1);
  });

  it("ignores a late frame from the socket it already replaced", () => {
    const { host, events, socket } = answering();
    socket.drop();
    vi.advanceTimersByTime(1000);
    const before = events.length;
    socket.receive({ type: "message", message: { id: "a", from: "x y", target: "z", state: "queued" } });
    socket.drop();
    expect(events.length).toBe(before);
    expect(reconnecting(events).filter((event) => event.type === "reconnecting" && event.retryAt !== null)).toHaveLength(1);
    host.close();
  });
});

describe("typed input that goes nowhere", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  const refused = (events: HostEvent[]) => events.filter((event) => event.type === "input_refused");
  const lastId = (socket: StubSocket) => (JSON.parse(socket.sent.at(-1)!) as { id: string }).id;

  it("says so in the daemon's words when an error answers typed input and no lock explains it", () => {
    const { host, events, socket } = connect();
    socket.receive({ ...WELCOME, typing: { allowed: true } });
    host.input("seat", "hello");
    socket.receive({ type: "error", id: lastId(socket), code: 1, error: 'not attached to "seat"' });
    expect(refused(events)).toEqual([{ type: "input_refused", text: 'Not sent. not attached to "seat"' }]);
    host.close();
  });

  it("leaves a refusal the lock already shows to the lock", () => {
    const { host, events, socket } = connect();
    socket.receive({ ...WELCOME, typing: { allowed: true } });
    host.input("seat", "hello");
    socket.receive({ type: "error", id: lastId(socket), code: 6, error: "assert this device's passkey before it types", reason: "passkey_required" });
    expect(refused(events)).toEqual([]);
    expect(typings(events).at(-1)).toEqual({ type: "typing", typing: { allowed: false, reason: "passkey_required" } });
    host.close();
  });

  it("ignores an error that names nothing typed", () => {
    const { host, events, socket } = connect();
    socket.receive({ ...WELCOME, typing: { allowed: true } });
    host.input("seat", "hello");
    socket.receive({ type: "error", id: "c999", code: 1, error: "unrelated" });
    expect(refused(events)).toEqual([]);
    host.close();
  });

  it("reports input lost to a closed socket instead of dropping it unseen", () => {
    vi.useFakeTimers();
    const { host, events, socket } = connect();
    socket.receive({ ...WELCOME, typing: { allowed: true } });
    socket.drop();
    const before = socket.sent.length;
    host.input("seat", "hello");
    expect(socket.sent.length).toBe(before);
    expect(refused(events)).toEqual([{ type: "input_refused", text: expect.stringMatching(/^Not sent\./) }]);
    host.close();
  });
});

describe("the passkey flag", () => {
  afterEach(() => vi.unstubAllGlobals());
  const flags = (events: HostEvent[]) => events.filter((event) => event.type === "passkey");

  it("says whether the welcome lists passkey, and replays it to a later subscriber", () => {
    const { host, events, socket } = connect();
    socket.receive({ ...WELCOME, features: ["typing-guard", "passkey"] });
    expect(flags(events)).toEqual([{ type: "passkey", available: true }]);
    const late: HostEvent[] = [];
    host.subscribe((event) => late.push(event));
    expect(flags(late)).toEqual([{ type: "passkey", available: true }]);
    host.close();
  });

  it("says no for a daemon that lists no passkey", () => {
    const { host, events, socket } = connect();
    socket.receive({ ...WELCOME, features: ["typing-guard"] });
    expect(flags(events)).toEqual([{ type: "passkey", available: false }]);
    host.close();
  });
});

describe("pty-size", () => {
  afterEach(() => vi.unstubAllGlobals());
  const SIZED = { ...WELCOME, features: ["pty-size"] };
  const frames = (socket: StubSocket) => socket.sent.map((line) => JSON.parse(line) as Record<string, unknown>);
  const attaches = (socket: StubSocket) => frames(socket).filter((frame) => frame.type === "attach" && frame.rows !== undefined);

  it("says a terminal pans only when the welcome lists pty-size", () => {
    const { host, socket } = connect();
    socket.receive(SIZED);
    host.attach("seat", 28, 47);
    expect(host.ptySize).toBe(true);
    expect(attaches(socket).at(-1)).toMatchObject({ rows: 28, cols: 47, scales: true });
    host.close();
  });

  it("keeps today's frames on a daemon that does not size the PTY", () => {
    const { host, socket } = connect();
    socket.receive(WELCOME);
    host.attach("seat", 28, 47);
    expect(host.ptySize).toBe(false);
    expect(attaches(socket).at(-1)).not.toHaveProperty("scales");
    host.close();
  });

  it("adds it to an attach queued before the welcome, once the welcome lists pty-size", () => {
    const { host, socket } = connect();
    host.attach("seat", 28, 47);
    expect(socket.sent).toEqual([]);
    socket.receive(SIZED);
    expect(attaches(socket)).toEqual([expect.objectContaining({ rows: 28, cols: 47, scales: true })]);
    host.close();
  });

  it("never claims a box for a seat that is only watched", () => {
    const { host, socket } = connect();
    socket.receive(SIZED);
    socket.receive({ type: "sessions", sessions: [{ name: "seat", role: "r", identity: "i", seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0 }] });
    const watch = frames(socket).find((frame) => frame.type === "attach" && frame.session === "seat");
    expect(watch).toMatchObject({ replay: false });
    expect(watch).not.toHaveProperty("scales");
    expect(watch).not.toHaveProperty("rows");
    host.close();
  });

  it("carries the box and the claim again when a terminal is re-attached after a redial", () => {
    vi.useFakeTimers();
    const { host, socket } = connect();
    socket.receive({ ...SIZED, features: ["pty-size"] });
    host.attach("seat", 28, 47);
    socket.drop();
    vi.advanceTimersByTime(60_000);
    const next = StubSocket.last;
    next.receive({ ...SIZED, features: ["pty-size"] });
    next.receive({ type: "sessions", sessions: [{ name: "seat", role: "r", identity: "i", seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0 }] });
    expect(attaches(next).at(-1)).toMatchObject({ rows: 28, cols: 47, scales: true, replay: true });
    host.close();
    vi.useRealTimers();
  });

  it("turns a size frame into an event for that seat, and ignores one with no size", () => {
    const { host, events, socket } = connect();
    socket.receive(SIZED);
    socket.receive({ type: "size", session: "seat", rows: 50, cols: 200 });
    socket.receive({ type: "size", session: "seat" });
    expect(events.filter((event) => event.type === "size")).toEqual([{ type: "size", sessionId: "seat", rows: 50, cols: 200 }]);
    host.close();
  });
});

describe("the device key", () => {
  afterEach(() => vi.unstubAllGlobals());

  const LOCKED = { allowed: false, reason: "passkey_required" };
  const FEATURES = { ...WELCOME, features: ["typing-guard", "device-key"] };
  const lastSent = (socket: StubSocket) => sentFrames(socket).at(-1)!;
  const kinds = (socket: StubSocket) => sentFrames(socket).map((frame) => frame.type);

  function plugin(overrides: Record<string, unknown> = {}) {
    const answers: Record<string, unknown> = {
      status: { enrolled: false, key_id: null, biometric: "ready" },
      enroll: { key_id: "k1", public_key: "PUB", attestation: ["LEAF", "INTERMEDIATE", "ROOT"] },
      sign: { key_id: "k1", signature: "SIG" },
      ...overrides,
    };
    const invoke = vi.fn(async (command: string) => {
      const answer = answers[command.replace("plugin:devicekey|", "")];
      if (answer instanceof Error) throw answer;
      return answer;
    });
    vi.stubGlobal("__TAURI__", { core: { invoke } });
    return invoke;
  }

  it("offers the channel only when the welcome lists device-key, and says so as an event", () => {
    const none = connect();
    none.socket.receive(WELCOME);
    expect(none.host.deviceKey).toBeUndefined();
    expect(none.events.filter((event) => event.type === "device_key")).toEqual([{ type: "device_key", available: false }]);
    none.host.close();

    const { host, events, socket } = connect();
    socket.receive(FEATURES);
    expect(host.deviceKey).toBeDefined();
    expect(events.filter((event) => event.type === "device_key")).toEqual([{ type: "device_key", available: true }]);
    host.close();
  });

  it("reads the device key standing off the typing frames, and keeps it when a refusal omits it", () => {
    const { host, events, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, device_key: "enrolled" } });
    socket.receive({ type: "error", id: "c9", code: 6, error: "refused", reason: "passkey_required" });
    const typing = events.filter((event) => event.type === "typing");
    expect(typing.at(-1)).toEqual({ type: "typing", typing: { ...LOCKED, deviceKey: "enrolled" } });
    host.close();
  });

  it("enrolls: asks the phone first, spends the code, enrolls on the challenge, finishes, then asserts at once", async () => {
    // After enrolling the phone reports its key, as the real plugin does.
    const invoke = plugin();
    invoke.mockImplementation(async (command: string) => {
      if (command.endsWith("status")) return enrolledNow ? { enrolled: true, key_id: "k1", biometric: "ready" } : { enrolled: false, key_id: null, biometric: "ready" };
      if (command.endsWith("enroll")) {
        enrolledNow = true;
        return { key_id: "k1", public_key: "PUB", attestation: ["LEAF", "INTERMEDIATE", "ROOT"] };
      }
      return { key_id: "k1", signature: "SIG" };
    });
    let enrolledNow = false;
    const { host, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, device_key: "unenrolled" } });
    const done = host.deviceKey!.enroll("ABCD-1234");
    await vi.waitFor(() => expect(lastSent(socket)).toMatchObject({ type: "device_enroll_begin", enroll_code: "ABCD-1234" }));
    expect(invoke.mock.calls[0]![0]).toBe("plugin:devicekey|status");
    socket.receive({ type: "device_enroll_challenge", id: lastSent(socket).id, challenge: "CHAL" });
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("device_enroll_finish"));
    expect(invoke).toHaveBeenCalledWith("plugin:devicekey|enroll", { challenge: "CHAL" });
    expect(lastSent(socket)).toMatchObject({ public_key: "PUB", attestation: ["LEAF", "INTERMEDIATE", "ROOT"] });
    socket.receive({ type: "device_enrolled", id: lastSent(socket).id, key_id: "k1" });
    await vi.waitFor(() => expect(lastSent(socket)).toMatchObject({ type: "device_assert_begin", key_id: "k1" }));
    socket.receive({ type: "device_assert_challenge", id: lastSent(socket).id, challenge: "CHAL2" });
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("device_assert_finish"));
    expect(invoke).toHaveBeenCalledWith("plugin:devicekey|sign", { challenge: "CHAL2", prompt: "Unlock aterm to type" });
    socket.receive({ type: "device_asserted", id: lastSent(socket).id });
    await expect(done).resolves.toBeUndefined();
    host.close();
  });

  it("keeps the key and says so when the prompt after enrolling is closed", async () => {
    let enrolledNow = false;
    const invoke = plugin();
    invoke.mockImplementation(async (command: string) => {
      if (command.endsWith("status")) return enrolledNow ? { enrolled: true, key_id: "k1", biometric: "ready" } : { enrolled: false, key_id: null, biometric: "ready" };
      if (command.endsWith("enroll")) {
        enrolledNow = true;
        return { key_id: "k1", public_key: "PUB", attestation: ["LEAF"] };
      }
      throw { code: "cancelled", message: "x" };
    });
    const { host, socket } = connect();
    socket.receive(FEATURES);
    const done = host.deviceKey!.enroll("ABCD-1234");
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("device_enroll_begin"));
    socket.receive({ type: "device_enroll_challenge", id: lastSent(socket).id, challenge: "CHAL" });
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("device_enroll_finish"));
    socket.receive({ type: "device_enrolled", id: lastSent(socket).id, key_id: "k1" });
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("device_assert_begin"));
    socket.receive({ type: "device_assert_challenge", id: lastSent(socket).id, challenge: "CHAL2" });
    await expect(done).rejects.toThrow(/closed the prompt.*Your key is set up/);
    host.close();
  });

  it("never sends a code for a phone that cannot ask for a fingerprint", async () => {
    plugin({ status: { enrolled: false, key_id: null, biometric: "none_enrolled" } });
    const { host, socket } = connect();
    socket.receive(FEATURES);
    await expect(host.deviceKey!.enroll("ABCD-1234")).rejects.toThrow(/Set up a fingerprint or screen lock/);
    expect(kinds(socket)).not.toContain("device_enroll_begin");
    host.close();
  });

  it("says the code is used up when the phone ends the enrollment after the daemon took it", async () => {
    plugin({ enroll: Object.assign(new Error("native"), { code: "cancelled" }) });
    const { host, socket } = connect();
    socket.receive(FEATURES);
    const done = host.deviceKey!.enroll("ABCD-1234");
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("device_enroll_begin"));
    socket.receive({ type: "device_enroll_challenge", id: lastSent(socket).id, challenge: "CHAL" });
    await expect(done).rejects.toThrow(/That code is used up/);
    host.close();
  });

  it("asserts: names its own key, signs the challenge behind the prompt, and finishes with the signature", async () => {
    const invoke = plugin({ status: { enrolled: true, key_id: "k1", biometric: "ready" } });
    const { host, socket } = connect();
    socket.receive({ ...FEATURES, typing: { ...LOCKED, device_key: "enrolled" } });
    const done = host.deviceKey!.assert();
    await vi.waitFor(() => expect(lastSent(socket)).toMatchObject({ type: "device_assert_begin", key_id: "k1" }));
    socket.receive({ type: "device_assert_challenge", id: lastSent(socket).id, challenge: "CHAL" });
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("device_assert_finish"));
    expect(invoke).toHaveBeenCalledWith("plugin:devicekey|sign", { challenge: "CHAL", prompt: "Unlock aterm to type" });
    expect(lastSent(socket)).toMatchObject({ key_id: "k1", signature: "SIG" });
    socket.receive({ type: "device_asserted", id: lastSent(socket).id });
    await expect(done).resolves.toBeUndefined();
    host.close();
  });

  it("says the host forgot this phone's key as a typed failure the panel can act on", async () => {
    plugin({ status: { enrolled: true, key_id: "k1", biometric: "ready" } });
    const { host, socket } = connect();
    socket.receive(FEATURES);
    const done = host.deviceKey!.assert();
    await vi.waitFor(() => expect(lastSent(socket).type).toBe("device_assert_begin"));
    socket.receive({ type: "error", id: lastSent(socket).id, code: 6, error: "no such key", reason: "device_key_unknown" });
    await expect(done).rejects.toMatchObject({ reason: "device_key_unknown", message: expect.stringMatching(/does not know this phone/) });
    host.close();
  });

  it("will not prompt a phone with no key yet, and says there is no plugin outside the app", async () => {
    plugin();
    const { host, socket } = connect();
    socket.receive(FEATURES);
    await expect(host.deviceKey!.assert()).rejects.toThrow(/no key yet/);
    expect(kinds(socket)).not.toContain("device_assert_begin");
    vi.unstubAllGlobals();
    await expect(host.deviceKey!.status()).rejects.toThrow(/no way to hold a key/);
    host.close();
  });
});

describe("a pane that never drew", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("still gets a frame when a listener before it throws", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const { host, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1" });
    host.subscribe(() => {
      throw new Error("an earlier listener broke");
    });
    const seen: HostEvent[] = [];
    host.subscribe((event) => seen.push(event));
    socket.receive({ type: "output", session: "s", data: btoa("hello") });
    expect(seen.filter((event) => event.type === "output")).toHaveLength(1);
    host.close();
  });

  it("asks again for the seat's screen with its own box and no new reference", () => {
    const { host, socket } = connect();
    socket.receive({ type: "welcome", format: "aterm.daemon.v1" });
    host.attach("s", 40, 120);
    socket.sent.length = 0;
    host.replay("s");
    expect(socket.sent.map((line) => JSON.parse(line))).toEqual([expect.objectContaining({ type: "attach", session: "s", replay: true, rows: 40, cols: 120 })]);
    host.detach("s");
    expect(socket.sent.map((line) => JSON.parse(line).type)).toContain("detach");
    host.close();
  });
});
