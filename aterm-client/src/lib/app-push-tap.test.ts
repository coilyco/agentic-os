import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// A tapped push names a brokered machine, which is listed only once the account signs in.
class NoSocket {
  addEventListener(): void {}
  send(): void {}
  close(): void {}
}

async function boot() {
  vi.stubEnv("DEV", false);
  vi.stubGlobal("WebSocket", NoSocket);
  vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost", pathname: "/" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {}, hasFocus: () => true });
  const app = await import("./app.svelte");
  const push = await import("./push.svelte");
  return { ...app, ...push };
}

const machine = (daemonId: string) => ({
  id: `broker:${daemonId}`,
  label: "kai-mac",
  address: "through the broker",
  kind: "daemon" as const,
  brokered: { daemonId, substrate: "mac" },
  status: { kind: "unreachable" as const, reason: "Offline." },
});

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetModules();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  vi.useRealTimers();
});

describe("a tapped push", () => {
  it("waits for its machine to be listed, then opens it", async () => {
    const { app, openPushTap, settlePushTap } = await boot();
    openPushTap({ kind: "ask", daemonId: "d1", session: "eng-platform-tender-beetle" });
    expect(app.selectedHostId, "nothing to open before sign-in lists the machine").toBeNull();
    app.hosts = [...app.hosts, machine("d1")];
    settlePushTap();
    expect(app.selectedHostId).toBe("broker:d1");
  });

  it("opens only the machine it names, and only once", async () => {
    const { app, openPushTap, settlePushTap } = await boot();
    app.hosts = [...app.hosts, machine("d1"), machine("d2")];
    openPushTap({ kind: "reachable", daemonId: "d2", session: null });
    expect(app.selectedHostId).toBe("broker:d2");
    app.selectedHostId = null;
    settlePushTap();
    expect(app.selectedHostId, "a settled tap does not pull the person back").toBeNull();
  });
});
