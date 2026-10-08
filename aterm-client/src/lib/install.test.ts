import { describe, expect, it } from "vitest";
import { installState, isStandalone } from "./install";

const base = { standalone: false, secure: true, prompt: false };

describe("installState", () => {
  it("hides once the page runs in its own window, whatever else is true", () => {
    expect(installState({ ...base, standalone: true, prompt: true })).toEqual({ kind: "installed" });
  });

  it("offers the browser's prompt when it gave one", () => {
    expect(installState({ ...base, prompt: true })).toEqual({ kind: "ready" });
  });

  it("points at the browser menu when a secure page got no prompt", () => {
    expect(installState(base)).toEqual({ kind: "menu" });
  });

  it("says why an insecure page cannot install, ahead of any prompt", () => {
    expect(installState({ ...base, secure: false, prompt: true })).toEqual({ kind: "insecure" });
  });
});

describe("isStandalone", () => {
  const only = (mode: string) => (query: string) => ({ matches: query === `(display-mode: ${mode})` });

  it("reads each installed display mode", () => {
    expect(isStandalone(only("standalone"), undefined)).toBe(true);
    expect(isStandalone(only("window-controls-overlay"), undefined)).toBe(true);
    expect(isStandalone(only("browser"), undefined)).toBe(false);
  });

  it("reads Safari's own flag for a home-screen app", () => {
    expect(isStandalone(only("browser"), true)).toBe(true);
  });
});
