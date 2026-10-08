import { describe, expect, it } from "vitest";
import { defaultSideTab, loadOverrides, parseOverrides, sideTabFor, storeOverride } from "./side-tab";

function memory(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial));
  return { getItem: (key: string) => data.get(key) ?? null, setItem: (key: string, value: string) => void data.set(key, value) };
}

describe("defaultSideTab", () => {
  it("opens a sysadmin on the terminal, whichever sysadmin it is", () => {
    expect(defaultSideTab("sysadmin-senior")).toBe("terminal");
    expect(defaultSideTab("sysadmin-access")).toBe("terminal");
    expect(defaultSideTab("sysadmin-junior")).toBe("terminal");
  });

  it("opens the frontend on the browser and the director on messages", () => {
    expect(defaultSideTab("frontend-eng")).toBe("browser");
    expect(defaultSideTab("prod-director")).toBe("messages");
  });

  it("opens every other role on messages", () => {
    for (const role of ["eng-platform", "scientist", "game-dev", "", "brand-new-role"]) expect(defaultSideTab(role)).toBe("messages");
  });

  it("matches a family by its dash, so a role that merely starts with the word is not in it", () => {
    expect(defaultSideTab("sysadmin")).toBe("messages");
    expect(defaultSideTab("frontendish")).toBe("messages");
    expect(defaultSideTab("prod-director-2")).toBe("messages");
  });
});

describe("side tab override", () => {
  it("wins over the role default, and only for the role it was set on", () => {
    expect(sideTabFor("sysadmin-senior", { "sysadmin-senior": "messages" })).toBe("messages");
    expect(sideTabFor("sysadmin-access", { "sysadmin-senior": "messages" })).toBe("terminal");
  });

  it("is remembered across a reload, one role at a time", () => {
    const storage = memory();
    storeOverride("frontend-eng", "terminal", storage);
    storeOverride("sysadmin-senior", "views", storage);
    expect(loadOverrides(storage)).toEqual({ "frontend-eng": "terminal", "sysadmin-senior": "views" });
  });

  it("replaces an earlier pick for the same role", () => {
    const storage = memory();
    storeOverride("frontend-eng", "terminal", storage);
    storeOverride("frontend-eng", "messages", storage);
    expect(loadOverrides(storage)).toEqual({ "frontend-eng": "messages" });
  });

  it("falls back to the defaults when storage is empty, missing, or throws", () => {
    expect(loadOverrides(memory())).toEqual({});
    expect(loadOverrides({ getItem: () => { throw new Error("blocked"); } })).toEqual({});
    expect(() => storeOverride("frontend-eng", "terminal", { getItem: () => null, setItem: () => { throw new Error("full"); } })).not.toThrow();
  });
});

describe("parseOverrides", () => {
  it("drops tabs this client does not know, such as one a later build wrote", () => {
    expect(parseOverrides(JSON.stringify({ a: "terminal", b: "sidebar", c: 3, d: null }))).toEqual({ a: "terminal" });
  });

  it("returns nothing for text that is not an object", () => {
    for (const raw of ["not json", "[]", "7", "null", ""]) expect(parseOverrides(raw)).toEqual({});
  });
});
