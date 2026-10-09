import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FoundHost } from "./discovery";

// app.svelte.ts dials hosts on import, so the browser globals it reads are stubbed
// first, and the two network calls it makes about hosts are scripted.
const replies: { found: FoundHost[]; answers: Record<string, boolean> } = { found: [], answers: {} };
const stored = new Map<string, string>();

vi.mock("./daemon-host", async (original) => ({
  ...(await original<typeof import("./daemon-host")>()),
  probe: vi.fn(async (address: string) => {
    if (replies.answers[address] === false) throw new Error("no answer");
    return 2;
  }),
}));
vi.mock("./reach", () => ({ diagnose: vi.fn(async () => ({ layer: "network", message: "Nothing answered." })) }));
vi.mock("./discovery", async (original) => ({
  ...(await original<typeof import("./discovery")>()),
  discover: vi.fn(async () => replies.found),
}));

async function boot() {
  vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  vi.stubGlobal("localStorage", { getItem: (key: string) => stored.get(key) ?? null, setItem: (key: string, value: string) => void stored.set(key, value) });
  const mod = await import("./app.svelte");
  await vi.advanceTimersByTimeAsync(0);
  return mod;
}

const tower: FoundHost = { name: "tower", host: "tower.example.ts.net", port: "7419" };
const ghost: FoundHost = { name: "ghost", host: "ghost.example.ts.net", port: "7419" };
const labels = (hosts: { label: string }[]) => hosts.map((host) => host.label);

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetModules();
  stored.clear();
  replies.found = [];
  replies.answers = { "wss://ghost.example.ts.net:7419/": false };
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("refreshing found hosts", () => {
  it("lists what a daemon sees, then drops a found host that never answered once no daemon lists it", async () => {
    replies.found = [tower, ghost];
    const { app } = await boot();
    expect(labels(app.hosts)).toEqual(["this Mac", "tower", "ghost", "Demo host"]);
    expect(app.hosts.find((host) => host.label === "ghost")?.status.kind).toBe("unreachable");

    replies.found = [tower];
    await vi.advanceTimersByTimeAsync(60_000);
    expect(labels(app.hosts)).toEqual(["this Mac", "tower", "Demo host"]);
  });

  it("keeps a found host that answered after the daemons stop listing it", async () => {
    replies.found = [tower];
    const { app } = await boot();
    replies.found = [];
    await vi.advanceTimersByTimeAsync(60_000);
    expect(labels(app.hosts)).toContain("tower");
  });

  it("drops nothing when no daemon replied", async () => {
    replies.found = [ghost];
    const { app } = await boot();
    const discover = (await import("./discovery")).discover as ReturnType<typeof vi.fn>;
    discover.mockRejectedValueOnce(new Error("no answer"));
    await vi.advanceTimersByTimeAsync(60_000);
    expect(labels(app.hosts)).toContain("ghost");
  });
});

describe("saving a found host", () => {
  it("stores it like a typed one, keeps the selection, and stops it being pruned", async () => {
    replies.found = [tower];
    const { app, saveFoundHost, selectHost } = await boot();
    const host = app.hosts.find((each) => each.label === "tower")!;
    selectHost(host);
    saveFoundHost(host);
    expect(host.id).toBe("saved:wss://tower.example.ts.net:7419/");
    expect(host.found).toBe(false);
    expect(app.selectedHostId).toBe(host.id);
    expect(app.attachedHostId).toBe(host.id);
    expect(JSON.parse(stored.get("aterm.hosts.v1")!)).toEqual([{ label: "tower", address: "wss://tower.example.ts.net:7419/" }]);

    replies.found = [];
    await vi.advanceTimersByTimeAsync(60_000);
    expect(labels(app.hosts)).toContain("tower");
  });

  it("is what typing a found host's name does", async () => {
    replies.found = [tower];
    const { app, addHost } = await boot();
    const added = addHost("tower.example.ts.net");
    expect(added.id.startsWith("saved:")).toBe(true);
    expect(app.hosts.filter((each) => each.label === "tower")).toHaveLength(1);
  });
});
