import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applicationServerKey } from "./push";

// 65 bytes of 0x04, as a daemon's uncompressed P-256 key is, in base64url.
const KEY = "BAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQ";
const ENDPOINT = "https://fcm.googleapis.com/fcm/send/abc";
const WELCOME = { type: "welcome", format: "aterm.daemon.v1" };

// A daemon that answers push_* the way the real one does, or refuses subscribe as locked.
class FakeDaemon {
  static all: FakeDaemon[] = [];
  static locked = false;
  sent: Record<string, unknown>[] = [];
  private handlers = new Map<string, ((event: unknown) => void)[]>();
  constructor() {
    FakeDaemon.all.push(this);
  }
  addEventListener(type: string, handler: (event: unknown) => void): void {
    this.handlers.set(type, [...(this.handlers.get(type) ?? []), handler]);
  }
  send(line: string): void {
    const frame = JSON.parse(line) as Record<string, unknown>;
    this.sent.push(frame);
    const id = frame.id as string;
    if (frame.type === "push_key") queueMicrotask(() => this.receive({ type: "push_key", id, key: KEY }));
    else if (frame.type === "push_subscribe" && FakeDaemon.locked) queueMicrotask(() => this.receive({ type: "error", id, code: 6, error: "refused", reason: "passkey_required" }));
    else if (frame.type === "push_subscribe") queueMicrotask(() => this.receive({ type: "push_subscribed", id }));
    else if (frame.type === "push_unsubscribe") queueMicrotask(() => this.receive({ type: "push_unsubscribed", id }));
  }
  close(): void {}
  receive(frame: Record<string, unknown>): void {
    for (const handler of this.handlers.get("message") ?? []) handler({ data: JSON.stringify(frame) });
  }
}

/** The browser's end: a permission, and the one subscription its push manager holds. */
function browser(permission: NotificationPermission = "default", answer: NotificationPermission = "granted") {
  const state = { permission, held: null as null | { endpoint: string; unsubscribe: ReturnType<typeof vi.fn> }, prompts: 0, subscribeError: null as Error | null };
  const manager = {
    getSubscription: async () => state.held,
    subscribe: async () => {
      if (state.subscribeError) throw state.subscribeError;
      state.held = {
        endpoint: ENDPOINT,
        unsubscribe: vi.fn(async () => ((state.held = null), true)),
        options: { applicationServerKey: applicationServerKey(KEY).buffer },
        toJSON: () => ({ endpoint: ENDPOINT, keys: { p256dh: "p", auth: "a" } }),
      } as never;
      return state.held;
    },
  };
  vi.stubGlobal("navigator", { serviceWorker: { ready: Promise.resolve({ pushManager: manager }) } });
  vi.stubGlobal("window", { PushManager: {}, addEventListener() {}, removeEventListener() {} });
  vi.stubGlobal("isSecureContext", true);
  const Notification = function () {};
  Object.defineProperty(Notification, "permission", { get: () => state.permission });
  Object.assign(Notification, {
    requestPermission: async () => {
      state.prompts++;
      state.permission = answer;
      return answer;
    },
  });
  vi.stubGlobal("Notification", Notification);
  return state;
}

async function attach(features: string[], typing?: object) {
  vi.stubGlobal("WebSocket", FakeDaemon);
  vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  const apps = await import("./app.svelte");
  const pushes = await import("./push.svelte");
  const host = apps.app.hosts.find((each) => each.kind === "daemon")!;
  host.status = { kind: "online", sessionCount: 0 };
  apps.selectHost(host);
  const daemon = FakeDaemon.all.at(-1)!;
  daemon.receive({ ...WELCOME, features, ...(typing ? { typing } : {}) });
  return { ...apps, ...pushes, host, daemon };
}

beforeEach(() => {
  vi.resetModules();
  vi.stubEnv("PROD", true);
  FakeDaemon.all = [];
  FakeDaemon.locked = false;
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

describe("a tapped notification", () => {
  const LOCAL = "ws://127.0.0.1:7419";
  const stored = (hosts: string[]) => vi.stubGlobal("localStorage", { getItem: () => JSON.stringify(hosts), setItem() {} });

  it("attaches the host this browser subscribed through, then opens the seat once it lists it", async () => {
    browser();
    stored([LOCAL]);
    vi.stubGlobal("WebSocket", FakeDaemon);
    vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
    vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
    const { app } = await import("./app.svelte");
    const { openSeatFromNotification, settleSeat } = await import("./push.svelte");
    const host = app.hosts.find((each) => each.address === LOCAL)!;
    host.status = { kind: "checking" };
    openSeatFromNotification("s1");
    // Still probing: it is picked, and attaches when the probe answers.
    expect(app.selectedHostId).toBe(host.id);
    expect(app.selectedSession).toBeNull();
    host.status = { kind: "online", sessionCount: 1 };
    const { selectHost } = await import("./app.svelte");
    selectHost(host);
    FakeDaemon.all.at(-1)!.receive({ ...WELCOME });
    FakeDaemon.all.at(-1)!.receive({ type: "sessions", sessions: [{ name: "s1", role: "frontend-eng", identity: "Imp-Dragonfly", seat: "claude", ready: true, bracketed_paste: true, kai_drafting: false, pending: 0 }] });
    settleSeat();
    expect(app.selectedSession).toBe("s1");
  });

  it("leaves a host alone that this browser never subscribed through", async () => {
    browser();
    stored([]);
    vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
    vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
    const { app } = await import("./app.svelte");
    const { openSeatFromNotification } = await import("./push.svelte");
    openSeatFromNotification("s1");
    expect(app.selectedHostId).toBeNull();
  });
});

describe("the Web Push switch", () => {
  it("is absent from a daemon whose welcome does not list web-push, and from a host that replaces it", async () => {
    browser();
    const { app, selectHost, host } = await attach(["terminals"]);
    expect(app.webPush).toBe(false);

    const withPush = await attach(["web-push"]);
    expect(withPush.app.webPush).toBe(true);
    // Moving to another host starts over: the switch belongs to the host that listed it.
    withPush.app.hosts.push({ id: "other", label: "other", address: "ws://other", kind: "daemon", status: { kind: "online", sessionCount: 0 } });
    selectHost(withPush.app.hosts.find((each) => each.id === "other")!);
    expect(withPush.app.webPush).toBe(false);
    void host;
  });

  it("starts off and reads off with nothing held, even where push is listed", async () => {
    browser();
    const { push, syncPush } = await attach(["web-push"]);
    await syncPush();
    expect(push).toMatchObject({ on: false, busy: false, message: "", failed: false });
  });

  it("turns on: permission, key, browser subscription, subscribe frame, then reads on and remembers the host", async () => {
    const page = browser("default");
    const { push, setPush, daemon, host, syncPush } = await attach(["web-push"]);
    await setPush(true);
    expect(page.prompts).toBe(1);
    expect(daemon.sent.map((frame) => frame.type)).toEqual(expect.arrayContaining(["push_key", "push_subscribe"]));
    expect(daemon.sent.find((frame) => frame.type === "push_subscribe")).toMatchObject({ push: { endpoint: ENDPOINT, keys: { p256dh: "p", auth: "a" } } });
    expect(push).toMatchObject({ on: true, busy: false, failed: false, message: "" });
    // A new page load reads the same state back from the browser and the stored host.
    await syncPush();
    expect(push.on).toBe(true);
    void host;
  });

  it("turns off: unsubscribe frame with the endpoint, browser subscription gone, reads off", async () => {
    const page = browser("granted");
    const { push, setPush, daemon } = await attach(["web-push"]);
    await setPush(true);
    await setPush(false);
    expect(daemon.sent.find((frame) => frame.type === "push_unsubscribe")).toMatchObject({ push: { endpoint: ENDPOINT } });
    expect(page.held).toBeNull();
    expect(push.on).toBe(false);
  });

  it("stays off and says the site is blocked, without asking the daemon", async () => {
    browser("denied");
    const { push, setPush, daemon } = await attach(["web-push"]);
    await setPush(true);
    expect(push).toMatchObject({ on: false, failed: true });
    expect(push.message).toMatch(/blocked for this site/);
    expect(daemon.sent.some((frame) => frame.type === "push_key")).toBe(false);
  });

  it("stays off and says the prompt was dismissed", async () => {
    browser("default", "default");
    const { push, setPush } = await attach(["web-push"]);
    await setPush(true);
    expect(push).toMatchObject({ on: false, failed: true });
    expect(push.message).toMatch(/nothing was turned on/);
  });

  it("explains a lock before it asks for permission, when the welcome already said locked", async () => {
    const page = browser("default");
    const { push, setPush, daemon } = await attach(["web-push", "passkey"], { allowed: false, reason: "passkey_required", passkey: "enrolled" });
    await setPush(true);
    expect(page.prompts).toBe(0);
    expect(push).toMatchObject({ on: false, failed: true });
    expect(push.message).toMatch(/hosted client at coilyco\.dev/);
    expect(daemon.sent.some((frame) => frame.type === "push_subscribe")).toBe(false);
  });

  it("explains a lock the daemon reports on subscribe, and leaves no browser subscription behind", async () => {
    const page = browser("granted");
    FakeDaemon.locked = true;
    const { push, setPush } = await attach(["web-push"]);
    await setPush(true);
    expect(push).toMatchObject({ on: false, failed: true });
    expect(push.message).toMatch(/locked/);
    expect(page.held).toBeNull();
  });

  it("says the push service was unreachable when the browser's subscribe fails", async () => {
    const page = browser("granted");
    page.subscribeError = Object.assign(new Error("Registration failed - push service error"), { name: "AbortError" });
    const { push, setPush } = await attach(["web-push"]);
    await setPush(true);
    expect(push).toMatchObject({ on: false, failed: true });
    expect(push.message).toMatch(/could not reach its push service/);
  });

  it("clears the last failure when the next try starts", async () => {
    const page = browser("denied");
    const { push, setPush } = await attach(["web-push"]);
    await setPush(true);
    expect(push.failed).toBe(true);
    page.permission = "granted";
    await setPush(true);
    expect(push).toMatchObject({ on: true, failed: false, message: "" });
  });

  it("reads off on a page that cannot receive push at all, such as a dev build", async () => {
    browser("granted");
    vi.stubEnv("PROD", false);
    const { push, syncPush, pushBlocked } = await attach(["web-push"]);
    expect(pushBlocked()).toMatch(/dev build/);
    await syncPush();
    expect(push.on).toBe(false);
  });

  it("reads off for a host this browser never subscribed through, even if the browser holds a subscription", async () => {
    const page = browser("granted");
    const { push, syncPush } = await attach(["web-push"]);
    await page.held;
    page.held = { endpoint: ENDPOINT, unsubscribe: vi.fn() };
    await syncPush();
    expect(push.on).toBe(false);
  });
});
