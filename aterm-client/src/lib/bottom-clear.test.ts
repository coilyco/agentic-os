import { readdirSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// An Android WebView reports no inset under its gesture bar, so taps there miss.
const root = new URL("../", import.meta.url);
const files = readdirSync(root, { recursive: true, encoding: "utf8" }).filter((path) => /\.(svelte|css)$/.test(path) && path !== "app.css");

describe("bottom clearance", () => {
  it("routes every bottom inset through --bottom-clear", () => {
    const raw = files.filter((path) => readFileSync(new URL(path, root), "utf8").includes("safe-area-inset-bottom"));
    expect(raw).toEqual([]);
  });

  it("keeps the token's floor in app.css", () => {
    expect(readFileSync(new URL("app.css", root), "utf8")).toMatch(/--bottom-clear: max\(env\(safe-area-inset-bottom, 0px\), \d+px\)/);
  });
});
