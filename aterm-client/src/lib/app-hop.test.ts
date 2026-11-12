import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AccountsError } from "./accounts";

// The sign-in hop through the app layer, with accounts and the browser scripted. What the
// client owns is what it opens, what it keeps, and what it does with the return.
const script = vi.hoisted(() => ({
  inApp: false,
  browser: true,
  openFails: "",
  handoffFails: false,
  exchanged: [] as { code: string; verifier: string }[],
  opened: [] as string[],
  returned: null as null | ((url: string) => void),
  closed: null as null | (() => void),
  cold: null as string | null,
  stopped: 0,
  signedOut: 0,
}));
const stored = new Map<string, string>();
const session = new Map<string, string>();

vi.mock("./build", async (original) => {
  const real = await original<typeof import("./build")>();
  return { ...real, get IN_APP() { return script.inApp; }, get EXTERNAL() { return true; } };
});
vi.mock("./treehouse", () => ({ treehouseConfig: () => ({ accountsUrl: "https://accounts.example.tech", brokerUrl: "https://broker.example.tech", hop: true }) }));
vi.mock("./system-browser", () => ({
  systemBrowser: () =>
    script.browser
      ? {
          open: async (url: string) => {
            if (script.openFails) throw new (await import("./handoff")).HandoffError(`This app could not open the sign-in page: ${script.openFails}`);
            script.opened.push(url);
          },
          takeReturn: async () => script.cold,
          onReturn: (listener: (url: string) => void) => {
            script.returned = listener;
            return () => void script.stopped++;
          },
          onClosed: (listener: () => void) => {
            script.closed = listener;
            return () => {};
          },
        }
      : null,
}));
vi.mock("./accounts", async (original) => {
  const real = await original<typeof import("./accounts")>();
  class FakeAccounts {
    session: { accountId: string; name: string; accessToken: string; accessExpiresAt: number } | null = null;
    async handoff(code: string, verifier: string) {
      script.exchanged.push({ code, verifier });
      if (script.handoffFails) throw new real.AccountsError("code_invalid", "that sign-in code is not valid", 400);
      this.session = { accountId: "acct_1", name: "kai", accessToken: "access-1", accessExpiresAt: Date.now() + 600_000 };
      return this.session;
    }
    async accessToken() {
      return "access-1";
    }
    async signOut() {
      script.signedOut++;
      this.session = null;
    }
  }
  return { ...real, AccountsClient: FakeAccounts };
});
vi.mock("./broker", async (original) => {
  const real = await original<typeof import("./broker")>();
  class FakeBroker {
    async daemons() {
      return [{ id: "dmn_01HZX3KA4B8N", name: "kai-server", substrate: "other", state: "online", channels: 0 }];
    }
    async connect() {
      return { url: "wss://broker.example/v1/channel/t", expiresIn: 60 };
    }
  }
  return { ...real, BrokerClient: FakeBroker };
});
vi.mock("./daemon-host", async (original) => ({ ...(await original<typeof import("./daemon-host")>()), probe: vi.fn(async () => 0) }));
vi.mock("./reach", () => ({ diagnose: vi.fn(async () => ({ layer: "network", message: "Nothing answered." })) }));
vi.mock("./discovery", async (original) => ({ ...(await original<typeof import("./discovery")>()), discover: vi.fn(async () => []) }));

const assign = vi.fn();
const replaceState = vi.fn();

async function boot(search = "") {
  vi.stubGlobal("location", { search, hostname: "coilyco.dev", protocol: "https:", host: "coilyco.dev", origin: "https://coilyco.dev", pathname: "/aterm/", href: `https://coilyco.dev/aterm/${search}`, assign });
  vi.stubGlobal("history", { replaceState });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  vi.stubGlobal("localStorage", { getItem: (key: string) => stored.get(key) ?? null, setItem: (key: string, value: string) => void stored.set(key, value), removeItem: (key: string) => void stored.delete(key) });
  vi.stubGlobal("sessionStorage", { getItem: (key: string) => session.get(key) ?? null, setItem: (key: string, value: string) => void session.set(key, value), removeItem: (key: string) => void session.delete(key) });
  vi.stubGlobal("WebSocket", class { addEventListener() {} send() {} close() {} });
  const mod = await import("./app.svelte");
  await vi.advanceTimersByTimeAsync(0);
  return mod;
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetModules();
  stored.clear();
  session.clear();
  assign.mockClear();
  replaceState.mockClear();
  Object.assign(script, { inApp: false, browser: true, openFails: "", handoffFails: false, exchanged: [], opened: [], returned: null, closed: null, cold: null, stopped: 0, signedOut: 0 });
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const startedUrl = () => new URL(assign.mock.calls[0]![0] as string);

describe("a page on the web", () => {
  it("goes to accounts' sign-in page with its own address as the return, keeping the verifier for the way back", async () => {
    const { app, signInThroughBrowser } = await boot();
    expect(app.accounts.hop).toBe(true);
    await signInThroughBrowser();
    const url = startedUrl();
    expect(url.origin + url.pathname).toBe("https://accounts.example.tech/signin");
    expect(url.searchParams.get("redirect_uri")).toBe("https://coilyco.dev/aterm/");
    expect(app.accounts.status).toBe("signing-in");
    const kept = JSON.parse(session.get("treehouse.handoff.v1")!);
    expect(kept.state).toBe(url.searchParams.get("state"));
    expect(url.toString()).not.toContain(kept.verifier);
  });

  it("signs in from the return, trades the code, lists the machines, and clears the query and what it kept", async () => {
    const first = await boot();
    await first.signInThroughBrowser();
    const kept = JSON.parse(session.get("treehouse.handoff.v1")!);
    vi.resetModules();
    const { app } = await boot(`?code=${"C".repeat(43)}&state=${kept.state}`);
    await vi.advanceTimersByTimeAsync(0);
    expect(script.exchanged).toEqual([{ code: "C".repeat(43), verifier: kept.verifier }]);
    expect(app.accounts).toMatchObject({ status: "signed-in", name: "kai", error: "" });
    expect(app.hosts.some((host) => host.id === "broker:dmn_01HZX3KA4B8N")).toBe(true);
    expect(replaceState).toHaveBeenCalledWith(null, "", "/aterm/");
    expect(session.has("treehouse.handoff.v1")).toBe(false);
  });

  it("refuses a return with a different state, in words, and exchanges nothing", async () => {
    const first = await boot();
    await first.signInThroughBrowser();
    vi.resetModules();
    const { app } = await boot(`?code=${"C".repeat(43)}&state=someone-elses`);
    await vi.advanceTimersByTimeAsync(0);
    expect(script.exchanged).toEqual([]);
    expect(app.accounts).toMatchObject({ status: "signed-out", error: "The sign-in did not come back as expected, so nothing was signed in. Try again." });
    expect(session.has("treehouse.handoff.v1")).toBe(false);
  });

  it("refuses a return it never asked for, such as a link someone made", async () => {
    const { app } = await boot(`?code=${"C".repeat(43)}&state=abc`);
    await vi.advanceTimersByTimeAsync(0);
    expect(script.exchanged).toEqual([]);
    expect(app.accounts.error).toMatch(/did not come back as expected/);
  });

  it("says so when accounts refuses the code, and offers the button again", async () => {
    script.handoffFails = true;
    const first = await boot();
    await first.signInThroughBrowser();
    const kept = JSON.parse(session.get("treehouse.handoff.v1")!);
    vi.resetModules();
    const { app } = await boot(`?code=${"C".repeat(43)}&state=${kept.state}`);
    await vi.advanceTimersByTimeAsync(0);
    expect(app.accounts).toMatchObject({ status: "signed-out", error: "that sign-in code is not valid" });
  });
});

describe("the app", () => {
  it("opens the sign-in page in the system browser with the app's return address, and finishes when the app is opened again", async () => {
    script.inApp = true;
    const { app, signInThroughBrowser } = await boot();
    await signInThroughBrowser();
    expect(assign).not.toHaveBeenCalled();
    const url = new URL(script.opened[0]!);
    expect(url.searchParams.get("redirect_uri")).toBe("dev.coilyco.treehouse:/signin");
    expect(app.accounts.status).toBe("signing-in");
    script.returned!(`dev.coilyco.treehouse:/signin?code=${"D".repeat(43)}&state=${url.searchParams.get("state")}`);
    await vi.advanceTimersByTimeAsync(0);
    expect(script.exchanged).toHaveLength(1);
    expect(app.accounts.status).toBe("signed-in");
    expect(script.stopped).toBe(1);
  });

  it("says so, and stays signed out, when this build cannot open the browser", async () => {
    script.inApp = true;
    script.browser = false;
    const { app, signInThroughBrowser } = await boot();
    await signInThroughBrowser();
    expect(app.accounts).toMatchObject({ status: "signed-out", error: "This app cannot open the sign-in page in your browser yet." });
    expect(script.opened).toEqual([]);
  });

  it("returns to the first card when the person gives up, and ignores a return that comes after", async () => {
    script.inApp = true;
    const { app, signInThroughBrowser, cancelSignIn } = await boot();
    await signInThroughBrowser();
    const state = new URL(script.opened[0]!).searchParams.get("state");
    cancelSignIn();
    expect(app.accounts).toMatchObject({ status: "signed-out", error: "" });
    script.returned?.(`dev.coilyco.treehouse:/signin?code=${"D".repeat(43)}&state=${state}`);
    await vi.advanceTimersByTimeAsync(0);
    expect(script.exchanged).toEqual([]);
    expect(app.accounts.status).toBe("signed-out");
  });

  it("keeps the verifier in local storage while the browser is open, and drops it once the code is traded", async () => {
    script.inApp = true;
    const { app, signInThroughBrowser } = await boot();
    await signInThroughBrowser();
    const url = new URL(script.opened[0]!);
    const kept = JSON.parse(stored.get("treehouse.handoff.v1")!);
    expect(kept.state).toBe(url.searchParams.get("state"));
    expect(session.has("treehouse.handoff.v1")).toBe(false);
    script.returned!(`dev.coilyco.treehouse:/signin?code=${"D".repeat(43)}&state=${kept.state}`);
    await vi.advanceTimersByTimeAsync(0);
    expect(app.accounts.status).toBe("signed-in");
    expect(stored.has("treehouse.handoff.v1")).toBe(false);
  });

  it("goes back to the first card, with nothing said, when the browser is dismissed with no return", async () => {
    script.inApp = true;
    const { app, signInThroughBrowser } = await boot();
    await signInThroughBrowser();
    script.closed!();
    expect(app.accounts).toMatchObject({ status: "signed-out", error: "" });
    expect(stored.has("treehouse.handoff.v1")).toBe(false);
  });

  it("gives up after ten minutes with no return, in words", async () => {
    script.inApp = true;
    const { app, signInThroughBrowser } = await boot();
    await signInThroughBrowser();
    await vi.advanceTimersByTimeAsync(9 * 60_000);
    expect(app.accounts.status).toBe("signing-in");
    await vi.advanceTimersByTimeAsync(2 * 60_000);
    expect(app.accounts).toMatchObject({ status: "signed-out", error: "The sign-in took too long. Try again." });
    expect(stored.has("treehouse.handoff.v1")).toBe(false);
  });

  it("says so, and stays signed out, when the plugin refuses the address", async () => {
    script.inApp = true;
    script.openFails = "the address is not one this app signs in on";
    const { app, signInThroughBrowser } = await boot();
    await signInThroughBrowser();
    expect(app.accounts).toMatchObject({ status: "signed-out", error: "This app could not open the sign-in page: the address is not one this app signs in on" });
    expect(stored.has("treehouse.handoff.v1")).toBe(false);
  });
});

describe("the app, started again while the browser was open", () => {
  const kept = (at = Date.now()) => JSON.stringify({ verifier: "V".repeat(43), state: "kept-state", at });

  it("signs in from the return the app was started with", async () => {
    script.inApp = true;
    stored.set("treehouse.handoff.v1", kept());
    script.cold = `dev.coilyco.treehouse:/signin?code=${"D".repeat(43)}&state=kept-state`;
    const { app } = await boot();
    await vi.advanceTimersByTimeAsync(0);
    expect(script.exchanged).toEqual([{ code: "D".repeat(43), verifier: "V".repeat(43) }]);
    expect(app.accounts).toMatchObject({ status: "signed-in", name: "kai" });
    expect(stored.has("treehouse.handoff.v1")).toBe(false);
  });

  it("drops a return it has no sign-in waiting for, and trades nothing", async () => {
    script.inApp = true;
    script.cold = `dev.coilyco.treehouse:/signin?code=${"D".repeat(43)}&state=kept-state`;
    const { app } = await boot();
    await vi.advanceTimersByTimeAsync(0);
    expect(script.exchanged).toEqual([]);
    expect(app.accounts).toMatchObject({ status: "signed-out", error: "" });
  });

  it("drops a verifier older than the time limit", async () => {
    script.inApp = true;
    stored.set("treehouse.handoff.v1", kept(Date.now() - 11 * 60_000));
    script.cold = `dev.coilyco.treehouse:/signin?code=${"D".repeat(43)}&state=kept-state`;
    const { app } = await boot();
    await vi.advanceTimersByTimeAsync(0);
    expect(script.exchanged).toEqual([]);
    expect(app.accounts.status).toBe("signed-out");
    expect(stored.has("treehouse.handoff.v1")).toBe(false);
  });

  it("goes on waiting when the return has not come yet, and finishes when it does", async () => {
    script.inApp = true;
    stored.set("treehouse.handoff.v1", kept());
    const { app } = await boot();
    expect(app.accounts.status).toBe("signing-in");
    script.returned!(`dev.coilyco.treehouse:/signin?code=${"D".repeat(43)}&state=kept-state`);
    await vi.advanceTimersByTimeAsync(0);
    expect(app.accounts.status).toBe("signed-in");
  });

  it("refuses a return whose state is not the kept one, in words", async () => {
    script.inApp = true;
    stored.set("treehouse.handoff.v1", kept());
    script.cold = `dev.coilyco.treehouse:/signin?code=${"D".repeat(43)}&state=someone-elses`;
    const { app } = await boot();
    await vi.advanceTimersByTimeAsync(0);
    expect(script.exchanged).toEqual([]);
    expect(app.accounts).toMatchObject({ status: "signed-out", error: "The sign-in did not come back as expected, so nothing was signed in. Try again." });
  });
});

void AccountsError;
