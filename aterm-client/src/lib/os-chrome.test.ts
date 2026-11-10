import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { tokenHex } from "../../pwa-plugin.ts";

const read = (path: string) => readFileSync(new URL(path, import.meta.url), "utf8");

describe("tokenHex", () => {
  it("reads a #rrggbb token from the block", () => expect(tokenHex(":root {\n  --ground: #0a0b0c;\n}", "--ground")).toBe("#0a0b0c"));
  it("does not take a longer token for the one asked", () => expect(tokenHex("--ground-x: #111111;\n--ground: #222222;", "--ground")).toBe("#222222"));
  it("refuses a token that is derived, since the OS cannot evaluate it", () => {
    expect(() => tokenHex("--ground: rgb(from red r g b);", "--ground")).toThrow(/--ground/);
  });
});

// Gradle reads the launcher ground as a resource, so the build cannot fill it.
describe("the Android launcher ground", () => {
  it("is the plate the manifest uses", () => {
    const launcher = /name="ic_launcher_background">(#[0-9a-fA-F]{6})</.exec(read("../../../aterm-android/src-tauri/icons/android/values/ic_launcher_background.xml"));
    expect(launcher?.[1]).toBe(tokenHex(read("../app.css"), "--plate"));
  });
});
