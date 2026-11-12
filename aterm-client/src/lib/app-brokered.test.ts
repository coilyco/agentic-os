import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AccountsError } from "./accounts";
import { BrokerError } from "./broker";

// app.svelte.ts dials hosts on import, so the browser globals it reads are stubbed first.
// Accounts and the broker are scripted. The treehouse-servers testbed uses the real ones.
const script = {
  configured: true,
  signIn: null as Error | null,
  daemons: [] as { id: string; name: string; substrate: string; state: "online" | "offline"; channels: number }[],
  listFails: null as Error | null,
  connectFails: null as Error | null,
  unlockForbidden: false,
  unlocked: [] as string[],
  ticket: "wss://broker.example/v1/channel/t_one",
  signedOut: 0,
  tickets: 0,
};
const stored = new Map<string, string>();

vi.mock("./treehouse", () => ({
  treehouseConfig: () => (script.configured ? { accountsUrl: "https://accounts.example.tech", brokerUrl: "https://broker.example.tech" } : null),
}));
vi.mock("./accounts", async (original) => {
  const real = await original<typeof import("./accounts")>();
  class FakeAccounts {
    session: { accountId: string; name: string; accessToken: string; accessExpiresAt: number } | null = null;
    async signIn() {
      if (script.signIn) throw script.signIn;
      this.session = { accountId: "acct_1", name: "kai", accessToken: "access-1", accessExpiresAt: Date.now() + 600_000 };
      return this.session;
    }
    async signUp(name: string) {
      this.session = { accountId: "acct_1", name, accessToken: "access-1", accessExpiresAt: Date.now() + 600_000 };
      return this.session;
    }
    async accessToken() {
      return "access-1";
    }
    async signOut() {
      script.signedOut++;
      this.session = null;
    }
    async unlockBySession() {
      if (script.unlockForbidden) throw new real.AccountsError("forbidden", "this session is not user-verified, so unlock with a passkey", 403);
      script.unlocked.push("session");
      return { token: "unlock-1", expiresIn: 120 };
    }
    async unlockByPasskey() {
      script.unlocked.push("passkey");
      return { token: "unlock-2", expiresIn: 120 };
    }
  }
  return { ...real, AccountsClient: FakeAccounts };
});
vi.mock("./broker", async (original) => {
  const real = await original<typeof import("./broker")>();
  class FakeBroker {
    async daemons() {
      if (script.listFails) throw script.listFails;
      return script.daemons.map((each) => ({ ...each }));
    }
    async connect() {
      if (script.connectFails) throw script.connectFails;
      script.tickets++;
      return { url: script.ticket, expiresIn: 60 };
    }
  }
  return { ...real, BrokerClient: FakeBroker };
});
vi.mock("./daemon-host", async (original) => ({
  ...(await original<typeof import("./daemon-host")>()),
  probe: vi.fn(async () => 0),
}));
vi.mock("./reach", () => ({ diagnose: vi.fn(async () => ({ layer: "network", message: "Nothing answered." })) }));
vi.mock("./discovery", async (original) => ({ ...(await original<typeof import("./discovery")>()), discover: vi.fn(async () => []) }));

class StubSocket {
  static all: StubSocket[] = [];
  sent: string[] = [];
  constructor(readonly url: string) {
    StubSocket.all.push(this);
  }
  private handlers: ((event: unknown) => void)[] = [];
  addEventListener(type: string, handler: (event: unknown) => void) {
    if (type === "message") this.handlers.push(handler);
  }
  send(line: string) {
    this.sent.push(line);
  }
  close() {}
  receive(frame: Record<string, unknown>) {
    for (const handler of this.handlers) handler({ data: JSON.stringify(frame) });
  }
}

async function boot() {
  vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  vi.stubGlobal("localStorage", { getItem: (key: string) => stored.get(key) ?? null, setItem: (key: string, value: string) => void stored.set(key, value) });
  vi.stubGlobal("WebSocket", StubSocket);
  const mod = await import("./app.svelte");
  await vi.advanceTimersByTimeAsync(0);
  return mod;
}

const mac = { id: "dmn_01HZX3KA4B8N", name: "kai-mac", substrate: "mac", state: "online" as const, channels: 1 };
const server = { id: "dmn_01HZX3KA4B8M", name: "kai-server", substrate: "other", state: "offline" as const, channels: 0 };
const brokerHosts = <T extends { id: string }>(hosts: T[]): T[] => hosts.filter((host) => host.id.startsWith("broker:"));

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetModules();
  stored.clear();
  StubSocket.all = [];
  Object.assign(script, { configured: true, signIn: null, daemons: [], listFails: null, connectFails: null, unlockForbidden: false, unlocked: [], ticket: "wss://broker.example/v1/channel/t_one", signedOut: 0, tickets: 0 });
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("a build without Treehouse addresses", () => {
  it("has no accounts surface and the same hosts as before", async () => {
    script.configured = false;
    const { app } = await boot();
    expect(app.accounts.configured).toBe(false);
    expect(app.hosts.map((host) => host.id)).toEqual(["local", "demo"]);
  });
});

describe("signed out, the first thing you see", () => {
  it("offers sign-in and lists no machine of an account", async () => {
    const { app } = await boot();
    expect(app.accounts).toMatchObject({ configured: true, status: "signed-out", name: "", error: "", listed: false });
    expect(brokerHosts(app.hosts)).toEqual([]);
  });

  it("says why when the passkey prompt fails, and stays signed out", async () => {
    script.signIn = new AccountsError("passkey", "The passkey prompt was closed or timed out.", 0);
    const { app, signIn } = await boot();
    await signIn();
    expect(app.accounts.status).toBe("signed-out");
    expect(app.accounts.error).toBe("The passkey prompt was closed or timed out.");
  });

  it("says accounts could not be reached, in the person's words", async () => {
    script.signIn = new AccountsError("network", "Accounts could not be reached.", 0);
    const { app, signIn } = await boot();
    await signIn();
    expect(app.accounts.error).toBe("Accounts could not be reached.");
  });
});

describe("signed in", () => {
  it("lists the account's machines, an online one first and an offline one marked as offline", async () => {
    script.daemons = [server, mac];
    const { app, signIn } = await boot();
    await signIn();
    expect(app.accounts).toMatchObject({ status: "signed-in", name: "kai", error: "", listed: true });
    const rows = brokerHosts(app.hosts) as typeof app.hosts;
    expect(rows.map((host) => [host.label, host.status.kind])).toEqual([["kai-mac", "online"], ["kai-server", "unreachable"]]);
    expect(rows[1]?.status).toMatchObject({ reason: "Offline. It is not connected to the broker." });
    expect(rows[0]).toMatchObject({ id: "broker:dmn_01HZX3KA4B8N", brokered: { daemonId: "dmn_01HZX3KA4B8N" } });
  });

  it("is listed and empty when the account has no machine, which differs from not yet asked", async () => {
    const { app, signIn } = await boot();
    await signIn();
    expect(app.accounts.listed).toBe(true);
    expect(brokerHosts(app.hosts)).toEqual([]);
  });

  it("keeps the last list and says the broker could not be reached when asking fails", async () => {
    script.daemons = [mac];
    const { app, signIn } = await boot();
    await signIn();
    script.listFails = new BrokerError("network", "The broker could not be reached.", 0);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(brokerHosts(app.hosts)).toHaveLength(1);
    expect(app.accounts.listError).toBe("The broker could not be reached.");
    script.listFails = null;
    await vi.advanceTimersByTimeAsync(30_000);
    expect(app.accounts.listError).toBe("");
  });

  it("notices a machine coming online without a reload", async () => {
    script.daemons = [{ ...server }];
    const { app, signIn } = await boot();
    await signIn();
    expect(brokerHosts(app.hosts)[0]?.status.kind).toBe("unreachable");
    script.daemons = [{ ...server, state: "online", channels: 0 }];
    await vi.advanceTimersByTimeAsync(30_000);
    expect(brokerHosts(app.hosts)[0]?.status.kind).toBe("online");
  });

  it("drops a machine from the list when the account no longer has it", async () => {
    script.daemons = [mac, server];
    const { app, signIn } = await boot();
    await signIn();
    script.daemons = [mac];
    await vi.advanceTimersByTimeAsync(30_000);
    expect(brokerHosts(app.hosts).map((host) => host.label)).toEqual(["kai-mac"]);
  });

  it("puts brokered machines before the demo host, which stays last", async () => {
    script.daemons = [mac];
    const { app, signIn } = await boot();
    await signIn();
    expect(app.hosts.at(-1)?.id).toBe("demo");
  });
});

describe("opening a machine through the broker", () => {
  it("fetches a ticket, dials the ticket URL, and says hello", async () => {
    script.daemons = [mac];
    const { app, signIn, selectHost } = await boot();
    await signIn();
    selectHost(brokerHosts(app.hosts)[0] as (typeof app.hosts)[number]);
    await vi.advanceTimersByTimeAsync(0);
    expect(app.attachedHostId).toBe("broker:dmn_01HZX3KA4B8N");
    expect(app.brokered).toBe(true);
    expect(script.tickets).toBe(1);
    expect(StubSocket.all.map((socket) => socket.url)).toEqual(["wss://broker.example/v1/channel/t_one"]);
  });

  it("does not attach to an offline machine, and says so", async () => {
    script.daemons = [server];
    const { app, signIn, selectHost } = await boot();
    await signIn();
    selectHost(brokerHosts(app.hosts)[0] as (typeof app.hosts)[number]);
    expect(app.attachedHostId).toBeNull();
    expect(script.tickets).toBe(0);
  });

  it("leaves a direct host unbrokered", async () => {
    const { app, selectHost } = await boot();
    selectHost(app.hosts[0] as (typeof app.hosts)[number]);
    await vi.advanceTimersByTimeAsync(0);
    expect(app.brokered).toBe(false);
  });
});

describe("signing out", () => {
  it("ends the session, drops the account's machines, and closes a brokered connection", async () => {
    script.daemons = [mac];
    const { app, signIn, signOut, selectHost } = await boot();
    await signIn();
    selectHost(brokerHosts(app.hosts)[0] as (typeof app.hosts)[number]);
    await vi.advanceTimersByTimeAsync(0);
    await signOut();
    expect(script.signedOut).toBe(1);
    expect(app.accounts).toMatchObject({ status: "signed-out", name: "", listed: false });
    expect(brokerHosts(app.hosts)).toEqual([]);
    expect(app.attachedHostId).toBeNull();
    expect(app.connection).toBeNull();
    expect(app.brokered).toBe(false);
  });
});

const WELCOME = {
  type: "welcome",
  format: "aterm.daemon.v1",
  features: ["unlock-token"],
  unlock: { daemon_id: "dmn_01HZX3KA4B8N", nonce: "n3Qk9mV2xT7pLw0aZc5RyA" },
  typing: { allowed: false, reason: "passkey_required" },
};

describe("unlocking through the account", () => {
  async function opened() {
    script.daemons = [mac];
    const mod = await boot();
    await mod.signIn();
    mod.selectHost(brokerHosts(mod.app.hosts)[0]!);
    await vi.advanceTimersByTimeAsync(0);
    return mod;
  }

  it("starts locked in the account's words, then sends the session's unlock token with no prompt", async () => {
    const { app } = await opened();
    StubSocket.all[0]!.receive(WELCOME);
    expect(app.typing).toEqual({ allowed: false, reason: "passkey_required" });
    await vi.advanceTimersByTimeAsync(0);
    expect(script.unlocked).toEqual(["session"]);
    const frame = JSON.parse(StubSocket.all[0]!.sent.find((line) => line.includes('"unlock"'))!);
    expect(frame).toMatchObject({ type: "unlock", unlock_token: "unlock-1" });
    StubSocket.all[0]!.receive({ type: "typing", id: frame.id, typing: { allowed: true } });
    expect(app.typing.allowed).toBe(true);
  });

  it("falls back to a fresh passkey when accounts says the session is no longer user-verified", async () => {
    script.unlockForbidden = true;
    const { app } = await opened();
    StubSocket.all[0]!.receive(WELCOME);
    await vi.advanceTimersByTimeAsync(0);
    expect(script.unlocked).toEqual(["passkey"]);
    expect(StubSocket.all[0]!.sent.some((line) => line.includes("unlock-2"))).toBe(true);
    expect(app.typing.allowed).toBe(false);
  });
});

describe("a brokered connection that cannot be opened", () => {
  it("ends the sign-in when the broker says the token is not accepted, and says so on the first screen", async () => {
    script.daemons = [mac];
    const mod = await boot();
    await mod.signIn();
    script.connectFails = new BrokerError("unauthorized", "the access token is not accepted", 401);
    script.listFails = new BrokerError("unauthorized", "the access token is not accepted", 401);
    mod.selectHost(brokerHosts(mod.app.hosts)[0]!);
    await vi.advanceTimersByTimeAsync(0);
    expect(mod.app.accounts).toMatchObject({ status: "signed-out", error: "Your sign-in ended. Sign in again." });
    expect(brokerHosts(mod.app.hosts)).toEqual([]);
    expect(mod.app.attachedHostId).toBeNull();
  });

  it("says the machine is offline, and keeps it listed, when the broker answers daemon_offline", async () => {
    script.daemons = [mac];
    const mod = await boot();
    await mod.signIn();
    script.connectFails = new BrokerError("daemon_offline", "the daemon is not connected", 503, 5);
    script.daemons = [{ ...mac, state: "offline", channels: 0 }];
    mod.selectHost(brokerHosts(mod.app.hosts)[0]!);
    await vi.advanceTimersByTimeAsync(0);
    const row = brokerHosts(mod.app.hosts)[0]!;
    expect(row.status).toMatchObject({ kind: "unreachable", reason: "Offline. It is not connected to the broker." });
    expect(mod.app.accounts.status).toBe("signed-in");
  });
});
