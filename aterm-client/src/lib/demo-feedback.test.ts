import { afterEach, describe, expect, it, vi } from "vitest";

// Importing the whole app state module takes longer than 5 s on a busy machine.
vi.setConfig({ testTimeout: 30_000 });

describe("feedback in the demo", () => {
  afterEach(() => vi.unstubAllGlobals());
  it("says it is off, not that the host needs a newer Treehouse", async () => {
    vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
    vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
    vi.stubGlobal("localStorage", { getItem: () => null, setItem() {}, removeItem() {} });
    const app = await import("./app.svelte");
    const feedback = await import("./feedback.svelte");
    app.enterDemo();
    expect(feedback.feedbackAvailability()).toEqual({ ok: false, reason: "Feedback is off in the demo." });
    app.leaveDemo();
    expect(feedback.feedbackAvailability()).toEqual({ ok: false, reason: "Connect to a host first." });
  });
  it("leaves Settings behind when the demo is left from it", async () => {
    const app = await import("./app.svelte");
    app.enterDemo();
    app.openSettings();
    expect(app.app.settingsOpen).toBe(true);
    app.leaveDemo();
    expect(app.app.settingsOpen).toBe(false);
  });
});
