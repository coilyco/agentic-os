import { beforeEach, describe, expect, it, vi } from "vitest";
import { BrokerClient, BrokerError } from "./broker";
import { DENIED, disableAppPush, enableAppPush, NOT_IN_THIS_APP, NOT_SET_UP, parseTap, refusal, reRegister, SIGNED_OUT, takeTap, type AppRegistrar } from "./app-push";

// The push plugin as the page sees it. `permission` is the notification plugin's answer.
function plugin(options: { permission?: boolean | null; allow?: boolean; register?: unknown; token?: string | null; tap?: unknown } = {}) {
  const calls: string[] = [];
  const core = {
    invoke: vi.fn(async (command: string) => {
      calls.push(command);
      if (command === "plugin:notification|is_permission_granted") return "permission" in options ? options.permission : true;
      if (command === "plugin:notification|request_permission") return options.allow === false ? "denied" : "granted";
      if (command === "plugin:push|register") {
        if (options.register instanceof Error) throw options.register;
        return options.register ?? { token: "tok:1", device: "Pixel 8" };
      }
      if (command === "plugin:push|token") return options.token === undefined ? "tok:1" : options.token;
      if (command === "plugin:push|take_tap") return options.tap ?? null;
      throw new Error(`unexpected ${command}`);
    }),
  };
  return { core, calls };
}

const registrar = () => {
  const seen: { register: [string, string][]; unregister: string[] } = { register: [], unregister: [] };
  const target: AppRegistrar = {
    register: async (token, device) => void seen.register.push([token, device]),
    unregister: async (token) => void seen.unregister.push(token),
  };
  return { target, seen };
};

const rejection = (code: string) => Object.assign(new Error(code), { code });

beforeEach(() => {
  globalThis.localStorage?.clear?.();
});

describe("turning it on", () => {
  it("refuses with the phone's settings named when notifications are denied, and registers nothing", async () => {
    const { core, calls } = plugin({ permission: false });
    const { target, seen } = registrar();
    await expect(enableAppPush(core, target)).rejects.toThrow(DENIED);
    expect(seen.register).toEqual([]);
    expect(calls).not.toContain("plugin:push|register");
  });

  it("asks once when never asked, and a no is the same refusal", async () => {
    const { core, calls } = plugin({ permission: null, allow: false });
    const { target } = registrar();
    await expect(enableAppPush(core, target)).rejects.toThrow(DENIED);
    expect(calls).toContain("plugin:notification|request_permission");
  });

  it("sends the plugin's token and device to the broker", async () => {
    const { core } = plugin({ permission: null });
    const { target, seen } = registrar();
    await enableAppPush(core, target);
    expect(seen.register).toEqual([["tok:1", "Pixel 8"]]);
  });

  it("names the device even when the plugin gives none, within the broker's 64", async () => {
    const { target, seen } = registrar();
    await enableAppPush(plugin({ register: { token: "tok:2", device: "" } }).core, target);
    await enableAppPush(plugin({ register: { token: "tok:3", device: "x".repeat(80) } }).core, target);
    expect(seen.register.map(([, device]) => device)).toEqual(["Android", "x".repeat(64)]);
  });

  it("passes a plugin refusal on for the switch to word", async () => {
    const { target, seen } = registrar();
    await expect(enableAppPush(plugin({ register: rejection("push_off") }).core, target)).rejects.toMatchObject({ code: "push_off" });
    await expect(enableAppPush(plugin({ register: { token: "" } }).core, target)).rejects.toMatchObject({ code: "no_token" });
    expect(seen.register).toEqual([]);
  });
});

describe("turning it off", () => {
  it("drops this phone's token at the broker", async () => {
    const { target, seen } = registrar();
    await disableAppPush(plugin().core, target);
    expect(seen.unregister).toEqual(["tok:1"]);
  });

  it("sends nothing when no token was ever made", async () => {
    const { target, seen } = registrar();
    await disableAppPush(plugin({ token: null }).core, target);
    expect(seen.unregister).toEqual([]);
  });
});

describe("each sign-in", () => {
  it("registers the token again without asking, and a failure is quiet", async () => {
    const { target, seen } = registrar();
    expect(await reRegister(plugin({ permission: null }).core, target)).toBe(true);
    expect(seen.register).toHaveLength(1);
    expect(await reRegister(plugin({ register: rejection("push_off") }).core, target)).toBe(false);
  });
});

describe("what the switch says", () => {
  it("says plainly that a phone build without the server's push is not set up, and a Mac cannot", () => {
    expect(refusal(rejection("push_off"), true)).toBe(NOT_SET_UP);
    expect(refusal(rejection("push_off"), false)).toBe(NOT_IN_THIS_APP);
    expect(NOT_SET_UP).toMatch(/not set up on this server.*while the app is open still work/);
    // A broker with no FCM credential, once COI-3777 has it refuse that way.
    expect(refusal(new BrokerError("push_off", "Notifications with the app closed are not set up on this server.", 503), true)).toBe(NOT_SET_UP);
  });

  it("asks for a sign-in when the session is gone, and passes the broker's own words on", () => {
    expect(refusal(new BrokerError("unauthorized", "token expired", 401), true)).toBe(SIGNED_OUT);
    expect(refusal(new BrokerError("forbidden", "registering an app needs a user-verified session", 403), true)).toBe("registering an app needs a user-verified session");
    expect(refusal(new BrokerError("network", "The broker could not be reached.", 0), true)).toBe("The broker could not be reached.");
  });

  it("never shows a raw exception", () => {
    expect(refusal(new TypeError("x is undefined"), true)).toBe("Notifications with the app closed could not be turned on. Try again.");
    expect(refusal(rejection("no_token"), true)).toMatch(/gave no token/);
  });
});

describe("a tapped push", () => {
  it("reads a seat tap and a machine tap, and drops anything else", () => {
    expect(parseTap({ kind: "ask", daemon_id: "d1", session: "eng-platform-tender-beetle", tag: "t" })).toEqual({ kind: "ask", daemonId: "d1", session: "eng-platform-tender-beetle" });
    expect(parseTap({ kind: "reachable", daemon_id: "d1", session: null, tag: "t" })).toEqual({ kind: "reachable", daemonId: "d1", session: null });
    expect(parseTap({ kind: "ask", session: "s" })).toBeNull();
    expect(parseTap(null)).toBeNull();
  });

  it("takes the launch tap once, and a failing plugin reads as no tap", async () => {
    expect(await takeTap(plugin({ tap: { kind: "prompt", daemon_id: "d2", session: "s", tag: "t" } }).core)).toMatchObject({ daemonId: "d2", session: "s" });
    expect(await takeTap({ invoke: async () => Promise.reject(new Error("no plugin")) })).toBeNull();
  });
});

describe("the broker's shapes", () => {
  it("registers as app-register.v1 and unregisters as app-unregister.v1, with the account's token", async () => {
    const sent: { url: string; method: string; auth: string; body: unknown }[] = [];
    const fetch = vi.fn(async (url: string, init: RequestInit) => {
      sent.push({ url, method: init.method!, auth: (init.headers as Record<string, string>).Authorization ?? "", body: JSON.parse(String(init.body)) });
      return new Response(null, { status: 204 });
    });
    const broker = new BrokerClient({ baseUrl: "https://broker.example/", accessToken: async () => "acc", fetch: fetch as unknown as typeof globalThis.fetch });
    await broker.registerApp("tok:1", "Pixel 8");
    await broker.unregisterApp("tok:1");
    expect(sent).toEqual([
      { url: "https://broker.example/v1/push/apps", method: "POST", auth: "Bearer acc", body: { platform: "android", token: "tok:1", device: "Pixel 8" } },
      { url: "https://broker.example/v1/push/apps/unregister", method: "POST", auth: "Bearer acc", body: { token: "tok:1" } },
    ]);
  });
});
