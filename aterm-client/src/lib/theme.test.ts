import { beforeEach, describe, expect, it } from "vitest";
import { loadThemeChoice, resolveTheme, storeThemeChoice } from "./theme";

const shelf = new Map<string, string>();
beforeEach(() => {
  shelf.clear();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: { getItem: (key: string) => shelf.get(key) ?? null, setItem: (key: string, value: string) => void shelf.set(key, value) },
  });
});

describe("the theme choice", () => {
  it("is System when nothing is stored", () => {
    expect(loadThemeChoice()).toBe("system");
  });
  it("comes back as it was stored", () => {
    storeThemeChoice("light");
    expect(loadThemeChoice()).toBe("light");
    storeThemeChoice("dark");
    expect(loadThemeChoice()).toBe("dark");
  });
  it("reads a value it does not know as System", () => {
    shelf.set("aterm.theme.v1", "sepia");
    expect(loadThemeChoice()).toBe("system");
  });
});

describe("which theme a choice resolves to", () => {
  it("follows the device on System", () => {
    expect(resolveTheme("system", true)).toBe("light");
    expect(resolveTheme("system", false)).toBe("dark");
  });
  it("ignores the device once a theme is chosen", () => {
    expect(resolveTheme("dark", true)).toBe("dark");
    expect(resolveTheme("light", false)).toBe("light");
  });
});
