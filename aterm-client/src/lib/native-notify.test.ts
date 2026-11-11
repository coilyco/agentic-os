import { describe, expect, it, vi } from "vitest";
import { askPermission, listenForTaps, nativeCore, nativeNotice, permissionOf, showNative, tappedSeat } from "./native-notify";
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
    expect(nativeNotice(note).options).toEqual({ title: "Decision needed", body: "Pick the order", extra: { session: "prod-director-griffin-goose" } });
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
    expect(open).toHaveBeenCalledTimes(1);
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
