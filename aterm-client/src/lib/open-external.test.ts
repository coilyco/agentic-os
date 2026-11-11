import { describe, expect, it, vi } from "vitest";
import { inAndroidApp, inApp, isArtifactUrl, openExternal } from "./open-external";

const ARTIFACT = "https://claude.ai/artifact/LfwfhS4hgtHYSAB5gXhKsj";

describe("isArtifactUrl", () => {
  it("is a claude.ai artifact and the code variant, and nothing else", () => {
    expect(isArtifactUrl(ARTIFACT)).toBe(true);
    expect(isArtifactUrl("https://claude.ai/code/artifact/0d3e9d6c-aaaa-4b2c-9c1e-0a5f3b7d1234")).toBe(true);
    expect(isArtifactUrl("https://www.claude.ai/artifact/abc")).toBe(true);
    expect(isArtifactUrl("https://claude.ai/chat/abc")).toBe(false);
    expect(isArtifactUrl("https://claude.ai/artifact/")).toBe(false);
    expect(isArtifactUrl("http://claude.ai/artifact/abc")).toBe(false);
    expect(isArtifactUrl("https://evil.example/claude.ai/artifact/abc")).toBe(false);
    expect(isArtifactUrl("https://claude.ai.evil.example/artifact/abc")).toBe(false);
    expect(isArtifactUrl("not a url")).toBe(false);
  });
});

describe("openExternal", () => {
  it("asks the app's opener with the address when the page is in the app", async () => {
    const invoke = vi.fn().mockResolvedValue(undefined);
    const tab = vi.fn();
    expect(await openExternal(ARTIFACT, { invoke, tab })).toEqual({ ok: true });
    expect(invoke).toHaveBeenCalledWith("plugin:opener|open_url", { url: ARTIFACT });
    expect(tab, "no tab is opened beside the app's own call").not.toHaveBeenCalled();
  });

  it("opens a tab in a browser", async () => {
    const tab = vi.fn();
    expect(await openExternal(ARTIFACT, { invoke: null, tab })).toEqual({ ok: true });
    expect(tab).toHaveBeenCalledWith(ARTIFACT);
  });

  it("says in words when the app could not open it, with the reason it gave", async () => {
    const invoke = vi.fn().mockRejectedValue({ message: "no app can open this" });
    const result = await openExternal(ARTIFACT, { invoke, tab: vi.fn() });
    expect(result.ok).toBe(false);
    expect(result).toMatchObject({ message: expect.stringContaining("claude.ai/artifact/LfwfhS4hgtHYSAB5gXhKsj did not open in the browser: no app can open this") });
    expect((result as { message: string }).message).toContain("The address is on the card to copy.");
  });

  it("says so for a failure with no reason, and never throws", async () => {
    const result = await openExternal(ARTIFACT, { invoke: vi.fn().mockRejectedValue(undefined), tab: vi.fn() });
    expect(result).toMatchObject({ ok: false, message: expect.stringContaining("did not open in the browser. The address") });
  });

  it("refuses anything but a web address, so the app is never asked to open a file or an app link", async () => {
    const invoke = vi.fn();
    for (const url of ["file:///etc/passwd", "javascript:alert(1)", "intent://x#Intent;end", "mailto:a@b.c", "nonsense"]) {
      expect(await openExternal(url, { invoke, tab: vi.fn() })).toMatchObject({ ok: false });
    }
    expect(invoke).not.toHaveBeenCalled();
  });
});

describe("inAndroidApp", () => {
  const phone = "Mozilla/5.0 (Linux; Android 15; Pixel 7) AppleWebKit/537.36";
  it("is the app on an Android phone", () => expect(inAndroidApp({ invoke: vi.fn() }, phone)).toBe(true));
  it("is not a browser on the phone, which has no app bridge", () => expect(inAndroidApp({ invoke: null }, phone)).toBe(false));
  it("is not the Mac app", () => expect(inAndroidApp({ invoke: vi.fn() }, "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")).toBe(false));
});

describe("inApp", () => {
  it("is whether the page has the app's bridge", () => {
    expect(inApp({ invoke: vi.fn() })).toBe(true);
    expect(inApp({ invoke: null })).toBe(false);
  });
});
