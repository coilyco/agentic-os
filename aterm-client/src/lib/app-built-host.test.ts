import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// COI-3476: an app build names its own host, so the shell seeds no storage. The page
// boots with a stubbed environment and store, on a fresh and on a used profile.
class Dial {
  static all: Dial[] = [];
  private handlers = new Map<string, ((event: unknown) => void)[]>();
  constructor(readonly url: string) {
    Dial.all.push(this);
  }
  addEventListener(type: string, handler: (event: unknown) => void): void {
    this.handlers.set(type, [...(this.handlers.get(type) ?? []), handler]);
  }
  send(): void {}
  close(): void {}
  private emit(type: string, event: unknown): void {
    for (const handler of this.handlers.get(type) ?? []) handler(event);
  }
  answer(): void {
    this.emit("message", { data: JSON.stringify({ type: "welcome", format: "aterm.daemon.v1", version: "0.540.0" }) });
    this.emit("message", { data: JSON.stringify({ type: "sessions", sessions: [] }) });
  }
}

const MAC = "ws://127.0.0.1:7419/";
const stored = new Map<string, string>();
const writes: string[] = [];

async function launched(env: Record<string, string> = { VITE_ATERM_APP: "1", VITE_ATERM_LOCAL_HOST: MAC, VITE_ATERM_LOCAL_LABEL: "this Mac" }) {
  for (const [key, value] of Object.entries(env)) vi.stubEnv(key, value);
  vi.stubGlobal("WebSocket", Dial);
  vi.stubGlobal("location", { search: "", hostname: "tauri.localhost", protocol: "tauri:", host: "tauri.localhost" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => stored.get(key) ?? null,
    setItem: (key: string, value: string) => {
      writes.push(key);
      stored.set(key, value);
    },
  });
  vi.stubGlobal("fetch", () => Promise.reject(new Error("offline")));
  return import("./app.svelte");
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(Math, "random").mockReturnValue(0.5);
  vi.resetModules();
  Dial.all = [];
  stored.clear();
  writes.length = 0;
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  vi.useRealTimers();
});

describe("an app build that names its host", () => {
  it("lists the host first on an empty profile and writes nothing to storage", async () => {
    const { app } = await launched();
    expect(app.hosts.map((host) => [host.label, host.address])).toEqual([
      ["this Mac", MAC],
      ["Demo host", "scripted, no daemon"],
    ]);
    expect(writes).toEqual([]);
  });

  it("rests on that host when nothing was last open, and attaches once it answers", async () => {
    const { app } = await launched();
    Dial.all.at(-1)!.answer();
    await vi.advanceTimersByTimeAsync(0);
    expect(app.selectedHostId).toBe(`saved:${MAC}`);
    expect(app.attachedHostId).toBe(`saved:${MAC}`);
    expect(app.selectedSession, "no seat is opened, the app rests on the overview").toBeNull();
  });

  it("does not list the host twice on a profile the old shell seeded", async () => {
    stored.set("aterm.hosts.v1", JSON.stringify([{ label: "this Mac", address: MAC }]));
    const { app } = await launched();
    expect(app.hosts.filter((host) => host.address === MAC)).toHaveLength(1);
  });

  it("stays removed once a person removed it", async () => {
    stored.set("aterm.hosts.v1", "[]");
    const { app } = await launched();
    expect(app.hosts.map((host) => host.label)).toEqual(["Demo host"]);
  });

  it("keeps the host a person last had open over its own", async () => {
    stored.set("aterm.hosts.v1", JSON.stringify([{ label: "tower", address: "wss://tower.example.ts.net/" }, { label: "this Mac", address: MAC }]));
    stored.set("aterm.last-open.v1", JSON.stringify({ host: "saved:wss://tower.example.ts.net/" }));
    const { app } = await launched();
    Dial.all.find((dial) => dial.url.includes("tower"))!.answer();
    await vi.advanceTimersByTimeAsync(0);
    expect(app.selectedHostId).toBe("saved:wss://tower.example.ts.net/");
  });
});

describe("builds that name no host", () => {
  it("a hosted or android build starts with none, as before", async () => {
    const { app } = await launched({ VITE_ATERM_APP: "1" });
    expect(app.hosts.map((host) => host.label)).toEqual(["Demo host"]);
  });

  it("a build that is not an app ignores the setting", async () => {
    const { app } = await launched({ VITE_ATERM_LOCAL_HOST: MAC });
    expect(app.hosts.some((host) => host.address === MAC && host.id.startsWith("saved:"))).toBe(false);
  });
});
