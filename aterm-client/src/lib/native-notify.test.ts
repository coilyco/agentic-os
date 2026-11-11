import { afterEach, describe, expect, it, vi } from "vitest";
import { askedBefore, askPermission, listenForTaps, mayAskNow, rememberAsked, nativeCore, nativeNotice, noticeId, parseNoted, permissionOf, seatOfTap, showNative, tappedFromActive, tappedSeat } from "./native-notify";
import type { Notify } from "./protocol";

const note: Notify = { id: "n1", session: "prod-director-griffin-goose", role: "prod-director", identity: "Griffin-Goose", title: "Decision needed", body: "Pick the order", at: 1 };
const core = (invoke: (command: string, args?: unknown) => Promise<unknown>) => ({ invoke });

describe("nativeCore", () => {
  it("is the bridge in the app and null in a browser", () => {
    const invoke = () => Promise.resolve(null);
    expect(nativeCore({ __TAURI__: { core: { invoke } } })).not.toBeNull();
    expect(nativeCore({})).toBeNull();
    expect(nativeCore({ __TAURI__: { core: {} } })).toBeNull();
  });
});

describe("permissionOf", () => {
  it("reads granted, denied, and never asked", async () => {
    expect(await permissionOf(core(() => Promise.resolve(true)))).toBe("granted");
    expect(await permissionOf(core(() => Promise.resolve(false)))).toBe("denied");
    expect(await permissionOf(core(() => Promise.resolve(null)))).toBe("ask");
  });
  it("reads a failed call as denied, so nothing is shown that the system may refuse", async () => {
    expect(await permissionOf(core(() => Promise.reject(new Error("no plugin"))))).toBe("denied");
  });
});

describe("askPermission", () => {
  it("is true only for granted", async () => {
    expect(await askPermission(core(() => Promise.resolve("granted")))).toBe(true);
    expect(await askPermission(core(() => Promise.resolve("denied")))).toBe(false);
    expect(await askPermission(core(() => Promise.resolve("prompt-with-rationale")))).toBe(false);
    expect(await askPermission(core(() => Promise.reject(new Error("x"))))).toBe(false);
  });
});

describe("showNative", () => {
  it("sends the seat's words with the seat in the extras", async () => {
    const invoke = vi.fn(() => Promise.resolve(null));
    expect(await showNative(core(invoke), note)).toBe(true);
    expect(invoke).toHaveBeenCalledWith("plugin:notification|notify", nativeNotice(note));
    expect(nativeNotice(note).options).toEqual({ id: noticeId(note.session), title: "Decision needed", body: "Pick the order", extra: { session: "prod-director-griffin-goose" } });
  });
  it("names the canopy as the small icon on Android and nowhere else", () => {
    expect(nativeNotice(note, true).options.icon).toBe("ic_stat_treehouse");
    expect(nativeNotice(note, false).options).not.toHaveProperty("icon");
  });
  it("is false when the system refuses", async () => {
    expect(await showNative(core(() => Promise.reject(new Error("refused"))), note)).toBe(false);
  });
});

describe("tappedSeat", () => {
  it("names the seat from the plugin's tap payload", () => {
    expect(tappedSeat({ actionId: "tap", notification: { extra: { session: "s1" } } })).toBe("s1");
  });
  it("names none for a tap with no extras, a non-string, an empty name, or junk", () => {
    expect(tappedSeat({ actionId: "tap", notification: {} })).toBeNull();
    expect(tappedSeat({ notification: { extra: { session: 4 } } })).toBeNull();
    expect(tappedSeat({ notification: { extra: { session: "" } } })).toBeNull();
    expect(tappedSeat(null)).toBeNull();
    expect(tappedSeat("tap")).toBeNull();
  });
});

describe("listenForTaps", () => {
  it("opens the seat of each tap and stops on request", async () => {
    let deliver: (payload: unknown) => void = () => {};
    const unregister = vi.fn(() => Promise.resolve());
    const add = vi.fn((_plugin: string, _event: string, callback: (payload: unknown) => void) => {
      deliver = callback;
      return Promise.resolve({ unregister });
    });
    const open = vi.fn();
    const stop = await listenForTaps({ invoke: () => Promise.resolve(null), addPluginListener: add }, open);
    expect(add).toHaveBeenCalledWith("notification", "actionPerformed", expect.any(Function));
    deliver({ notification: { extra: { session: "s1" } } });
    deliver({ notification: {} });
    await vi.waitFor(() => expect(open).toHaveBeenCalledTimes(1));
    expect(open).toHaveBeenCalledWith("s1");
    stop();
    expect(unregister).toHaveBeenCalled();
  });
  it("does nothing where the bridge cannot listen", async () => {
    const stop = await listenForTaps({ invoke: () => Promise.resolve(null) }, vi.fn());
    expect(() => stop()).not.toThrow();
    const failing = await listenForTaps({ invoke: () => Promise.resolve(null), addPluginListener: () => Promise.reject(new Error("x")) }, vi.fn());
    expect(() => failing()).not.toThrow();
  });
});

const storage = () => {
  const held = new Map<string, string>();
  return { getItem: (key: string) => held.get(key) ?? null, setItem: (key: string, value: string) => void held.set(key, value) };
};

afterEach(() => vi.unstubAllGlobals());

describe("noticeId", () => {
  it("is one positive whole number per seat, and differs between seats", () => {
    const id = noticeId("prod-director-griffin-goose");
    expect(id).toBe(noticeId("prod-director-griffin-goose"));
    expect(Number.isInteger(id) && id > 0 && id <= 2147483647).toBe(true);
    expect(id).not.toBe(noticeId("eng-platform-beetle-ox"));
  });
});

describe("parseNoted", () => {
  const now = 10 * 24 * 3600 * 1000;
  it("keeps what is under a day old and drops the rest, junk and wrong shapes", () => {
    const text = JSON.stringify({ "1": { session: "a", at: now - 1000 }, "2": { session: "b", at: now - 25 * 3600 * 1000 }, "3": { session: 4, at: now }, "4": null });
    expect(parseNoted(text, now)).toEqual({ "1": { session: "a", at: now - 1000 } });
    expect(parseNoted(null, now)).toEqual({});
    expect(parseNoted("not json", now)).toEqual({});
  });
});

describe("tappedFromActive", () => {
  const noted = { "10": { session: "old", at: 1 }, "20": { session: "new", at: 2 } };
  it("is the newest raised notification that is no longer showing", () => {
    expect(tappedFromActive(noted, new Set([10]))).toEqual({ id: "20", session: "new" });
    expect(tappedFromActive(noted, new Set([20]))).toEqual({ id: "10", session: "old" });
    expect(tappedFromActive(noted, new Set())).toEqual({ id: "20", session: "new" });
  });
  it("is none while every raised notification is still showing", () => {
    expect(tappedFromActive(noted, new Set([10, 20]))).toBeNull();
    expect(tappedFromActive({}, new Set())).toBeNull();
  });
});

describe("a tap that names no seat", () => {
  it("finds it from the record and what still shows, and forgets that notification", async () => {
    vi.stubGlobal("localStorage", storage());
    const sent = vi.fn(() => Promise.resolve(null));
    await showNative(core(sent), note, 1000);
    const active = vi.fn(() => Promise.resolve([] as unknown[]));
    const asked = (command: string) => (command === "plugin:notification|get_active" ? active() : Promise.resolve(null));
    expect(await seatOfTap(core(asked), { actionId: "tap" }, 2000)).toBe(note.session);
    // The same tap seen again has nothing left to open.
    expect(await seatOfTap(core(asked), { actionId: "tap" }, 3000)).toBeNull();
  });
  it("opens nothing when the notification is still showing, or the list cannot be read", async () => {
    vi.stubGlobal("localStorage", storage());
    await showNative(core(() => Promise.resolve(null)), note, 1000);
    const showing = () => Promise.resolve([{ id: noticeId(note.session) }]);
    expect(await seatOfTap(core(showing), { actionId: "tap" }, 2000)).toBeNull();
    expect(await seatOfTap(core(() => Promise.reject(new Error("no"))), { actionId: "tap" }, 2000)).toBeNull();
  });
  it("prefers a seat the payload names", async () => {
    expect(await seatOfTap(core(() => Promise.resolve(null)), { notification: { extra: { session: "named" } } })).toBe("named");
  });
});

describe("asking once", () => {
  afterEach(() => vi.unstubAllGlobals());
  const store = () => {
    const kept = new Map<string, string>();
    vi.stubGlobal("localStorage", { getItem: (key: string) => kept.get(key) ?? null, setItem: (key: string, value: string) => void kept.set(key, value) });
  };
  it("remembers that the prompt was shown, so a first no is the last ask", () => {
    store();
    expect(askedBefore()).toBe(false);
    rememberAsked();
    expect(askedBefore()).toBe(true);
  });
  it("reads as never asked, and does not throw, when storage is off", () => {
    vi.stubGlobal("localStorage", { getItem: () => { throw new Error("off"); }, setItem: () => { throw new Error("off"); } });
    expect(askedBefore()).toBe(false);
    expect(() => rememberAsked()).not.toThrow();
  });
  it("waits for a screen with no composer, a visible page, a host, and no demo", () => {
    const ready = { attached: true, visible: true, composerShown: false, demo: false };
    expect(mayAskNow(ready)).toBe(true);
    expect(mayAskNow({ ...ready, composerShown: true })).toBe(false);
    expect(mayAskNow({ ...ready, attached: false })).toBe(false);
    expect(mayAskNow({ ...ready, visible: false })).toBe(false);
    expect(mayAskNow({ ...ready, demo: true })).toBe(false);
  });
});
