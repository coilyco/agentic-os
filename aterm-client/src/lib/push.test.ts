import { describe, expect, it, vi } from "vitest";
import type { PushChannel } from "./protocol";
import { applicationServerKey, DENIED, disablePush, enablePush, lockedMessage, parsePushHosts, pushIsOn, pushUnavailable, PushError, resolveSeat, seatFromLocation, seatTag, subscribeFailure, type PushBrowser, type PushGlobals } from "./push";
import { Refusal } from "./typing";

const READY: PushGlobals = { secure: true, production: true, worker: true, manager: true, notification: true };
// 65 bytes of 0x04..., as a daemon's uncompressed P-256 key is, in base64url.
const KEY = "BAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQ";

function subscription(key = KEY, endpoint = "https://fcm.googleapis.com/fcm/send/abc") {
  return {
    endpoint,
    options: { applicationServerKey: applicationServerKey(key).buffer },
    toJSON: () => ({ endpoint, keys: { p256dh: "p", auth: "a" } }),
    unsubscribe: vi.fn().mockResolvedValue(true),
  };
}

function channel(overrides: Partial<PushChannel> = {}): PushChannel & { [K in keyof PushChannel]: ReturnType<typeof vi.fn> } {
  return {
    key: vi.fn().mockResolvedValue(KEY),
    subscribe: vi.fn().mockResolvedValue(undefined),
    unsubscribe: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  } as never;
}

function browser(permission: NotificationPermission, held: ReturnType<typeof subscription> | null = null, requested: NotificationPermission = "granted") {
  const made = subscription();
  const calls: string[] = [];
  const stub = {
    calls,
    made,
    permission: () => permission,
    requestPermission: vi.fn(async () => {
      calls.push("request");
      return requested;
    }),
    existing: vi.fn(async () => held),
    subscribe: vi.fn(async () => {
      calls.push("subscribe");
      return made as unknown as PushSubscription;
    }),
  };
  return stub as typeof stub & PushBrowser;
}

describe("whether this page can receive push at all", () => {
  it("is ready on a secure production page with a worker, a push manager, and notifications", () => {
    expect(pushUnavailable(READY)).toBeNull();
  });

  it("says why an insecure page cannot, before anything else", () => {
    expect(pushUnavailable({ ...READY, secure: false, worker: false })).toMatch(/secure page/);
  });

  it("points an iPhone at the Home Screen when the browser has no push manager", () => {
    expect(pushUnavailable({ ...READY, manager: false })).toMatch(/Home Screen/);
    expect(pushUnavailable({ ...READY, notification: false })).toMatch(/Home Screen/);
  });

  it("names a dev build, since no worker will ever become ready there", () => {
    expect(pushUnavailable({ ...READY, production: false })).toMatch(/dev build/);
  });
});

describe("the lock a remote device is under", () => {
  it("sends the hosted client to unlock, and a daemon-served one to coilyco.dev", () => {
    expect(lockedMessage(true)).toMatch(/Unlock it with a passkey/);
    expect(lockedMessage(false)).toMatch(/only work from the hosted client at coilyco\.dev/);
  });
});

describe("turning push on", () => {
  it("asks permission first, then the key, then subscribes the browser, then tells the daemon", async () => {
    const order: string[] = [];
    const daemon = channel({ key: vi.fn(async () => (order.push("key"), KEY)), subscribe: vi.fn(async () => void order.push("daemon")) });
    const page = browser("default");
    page.subscribe.mockImplementation(async () => (order.push("browser"), page.made as unknown as PushSubscription));
    page.requestPermission.mockImplementation(async () => (order.push("permission"), "granted" as const));
    await enablePush(daemon, page, true);
    expect(order).toEqual(["permission", "key", "browser", "daemon"]);
    expect(page.subscribe).toHaveBeenCalledWith(applicationServerKey(KEY));
    expect(daemon.subscribe).toHaveBeenCalledWith({ endpoint: "https://fcm.googleapis.com/fcm/send/abc", keys: { p256dh: "p", auth: "a" } });
  });

  it("does not ask again when permission is already granted", async () => {
    const page = browser("granted");
    await enablePush(channel(), page, true);
    expect(page.requestPermission).not.toHaveBeenCalled();
  });

  it("reuses a subscription that already carries the daemon's key, and sends it again", async () => {
    const held = subscription();
    const daemon = channel();
    const page = browser("granted", held);
    await enablePush(daemon, page, true);
    expect(page.subscribe).not.toHaveBeenCalled();
    expect(held.unsubscribe).not.toHaveBeenCalled();
    expect(daemon.subscribe).toHaveBeenCalledOnce();
  });

  it("drops a subscription made under another key and subscribes fresh", async () => {
    const other = "BQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQ";
    const held = subscription(other);
    const page = browser("granted", held);
    await enablePush(channel(), page, true);
    expect(held.unsubscribe).toHaveBeenCalledOnce();
    expect(page.subscribe).toHaveBeenCalledOnce();
  });

  it("stops at a blocked site without calling the daemon", async () => {
    const daemon = channel();
    await expect(enablePush(daemon, browser("denied"), true)).rejects.toThrow(DENIED);
    expect(daemon.key).not.toHaveBeenCalled();
  });

  it("says nothing was turned on when the prompt is dismissed", async () => {
    const daemon = channel();
    const error = await enablePush(daemon, browser("default", null, "default"), true).catch((each: unknown) => each);
    expect(error).toBeInstanceOf(PushError);
    expect((error as Error).message).toMatch(/nothing was turned on/);
    expect(daemon.key).not.toHaveBeenCalled();
  });

  it("reads a prompt answered no as blocked", async () => {
    await expect(enablePush(channel(), browser("default", null, "denied"), true)).rejects.toThrow(DENIED);
  });

  it("says the push service was unreachable when the browser's subscribe fails, and does not tell the daemon", async () => {
    const daemon = channel();
    const page = browser("granted");
    page.subscribe.mockRejectedValue(Object.assign(new Error("Registration failed - push service error"), { name: "AbortError" }));
    await expect(enablePush(daemon, page, true)).rejects.toThrow(/could not reach its push service/);
    expect(daemon.subscribe).not.toHaveBeenCalled();
  });

  it("explains a locked device in the hosted client's terms, and takes the new browser subscription back down", async () => {
    const daemon = channel({ subscribe: vi.fn().mockRejectedValue(new Refusal("locked", "passkey_required")) });
    const page = browser("granted");
    await expect(enablePush(daemon, page, true)).rejects.toThrow(lockedMessage(true));
    expect(page.made.unsubscribe).toHaveBeenCalledOnce();
  });

  it("explains a locked device on a daemon-served page as needing coilyco.dev", async () => {
    const daemon = channel({ subscribe: vi.fn().mockRejectedValue(new Refusal("locked", "passkey_required")) });
    await expect(enablePush(daemon, browser("granted"), false)).rejects.toThrow(lockedMessage(false));
  });

  it("keeps a held subscription when the daemon refuses, since it was not made here", async () => {
    const held = subscription();
    const daemon = channel({ subscribe: vi.fn().mockRejectedValue(new Error("push endpoint must be https")) });
    await expect(enablePush(daemon, browser("granted", held), true)).rejects.toThrow("push endpoint must be https");
    expect(held.unsubscribe).not.toHaveBeenCalled();
  });
});

describe("turning push off", () => {
  it("tells the daemon the endpoint, then drops the browser's subscription", async () => {
    const held = subscription();
    const daemon = channel();
    await disablePush(daemon, browser("granted", held));
    expect(daemon.unsubscribe).toHaveBeenCalledWith("https://fcm.googleapis.com/fcm/send/abc");
    expect(held.unsubscribe).toHaveBeenCalledOnce();
  });

  it("still drops the browser's subscription when the daemon cannot be told", async () => {
    const held = subscription();
    const daemon = channel({ unsubscribe: vi.fn().mockRejectedValue(new Refusal("locked", "passkey_required")) });
    await disablePush(daemon, browser("granted", held));
    expect(held.unsubscribe).toHaveBeenCalledOnce();
  });

  it("does nothing when the browser holds no subscription", async () => {
    const daemon = channel();
    await disablePush(daemon, browser("granted", null));
    expect(daemon.unsubscribe).not.toHaveBeenCalled();
  });
});

describe("what the switch reads", () => {
  it("is on only for a host this browser subscribed through and still holds a subscription for", () => {
    expect(pushIsOn(true, ["ws://a"], "ws://a")).toBe(true);
    expect(pushIsOn(true, ["ws://a"], "ws://b")).toBe(false);
    expect(pushIsOn(false, ["ws://a"], "ws://a")).toBe(false);
    expect(pushIsOn(true, ["ws://a"], undefined)).toBe(false);
  });

  it("reads stored hosts as none when missing or malformed", () => {
    expect(parsePushHosts(null)).toEqual([]);
    expect(parsePushHosts("{")).toEqual([]);
    expect(parsePushHosts('{"a":1}')).toEqual([]);
    expect(parsePushHosts('["ws://a",3,"ws://b"]')).toEqual(["ws://a", "ws://b"]);
  });

  it("reads a browser's NotAllowedError as blocked", () => {
    expect(subscribeFailure(Object.assign(new Error("x"), { name: "NotAllowedError" }))).toBe(DENIED);
  });
});

describe("a tapped notification", () => {
  it("names its seat in the launch address, and tags match the daemon's", () => {
    expect(seatFromLocation("?seat=frontend-eng-1")).toBe("frontend-eng-1");
    expect(seatFromLocation("?seat=")).toBeNull();
    expect(seatFromLocation("")).toBeNull();
    expect(seatTag("frontend-eng-1")).toBe("aterm-frontend-eng-1");
  });

  it("waits for a host, opens the seat once it is listed, and gives up after its wait", () => {
    const pending = { session: "s1", until: null };
    expect(resolveSeat(pending, false, [], 0)).toEqual({ select: null, keep: pending });
    const armed = resolveSeat(pending, true, [], 1000).keep!;
    expect(armed.until).toBeGreaterThan(1000);
    expect(resolveSeat(armed, true, ["s1"], 1500)).toEqual({ select: "s1", keep: null });
    expect(resolveSeat(armed, true, [], armed.until! + 1)).toEqual({ select: null, keep: null });
    // A seat that turns up after the wait is not a tap anymore.
    expect(resolveSeat(armed, true, ["s1"], armed.until! + 1)).toEqual({ select: null, keep: null });
  });

  it("opens a seat that is already listed without waiting", () => {
    expect(resolveSeat({ session: "s1", until: null }, true, ["s1", "s2"], 0)).toEqual({ select: "s1", keep: null });
  });
});
