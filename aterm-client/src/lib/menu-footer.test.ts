import { readdirSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { detectChoice } from "./choices";
import { menuFooterShowing } from "./menu-footer";

const read = (url: URL): string[] => readFileSync(url, "utf8").replace(/\n+$/, "").split("\n");
const screens = new URL("../../../aterm/testdata/claude-screens/", import.meta.url);
const captures = new URL("../../testbed/fixtures/captures/", import.meta.url);
const menus = (dir: URL): string[] => readdirSync(dir).filter((name) => name.endsWith(".txt"));

describe("a menu footer on the last rows (COI-3689)", () => {
  // The review screen draws no footer, and the card reader takes it by its Submit row.
  it("or the card reader sees every captured Claude Code menu", () => {
    const seen = (rows: string[]): boolean => menuFooterShowing(rows) || detectChoice(rows) !== null;
    for (const name of menus(screens).filter((each) => each !== "idle-ready.txt")) {
      expect(seen(read(new URL(name, screens))), name).toBe(true);
    }
    for (const name of menus(captures).filter((each) => /^claude-2\.1\.31/.test(each))) {
      expect(seen(read(new URL(name, captures))), name).toBe(true);
    }
  });

  it("is not showing at an idle prompt", () => {
    expect(menuFooterShowing(read(new URL("idle-ready.txt", screens)))).toBe(false);
  });

  it("is not showing when a reply above the input box says it", () => {
    const reply = ["● Press Esc to cancel, or Enter to select one.", "", "", "", "────────", "❯ ", "────────", "  ⏵⏵ auto mode on (shift+tab to cycle)"];
    expect(menuFooterShowing(reply)).toBe(false);
  });

  it("is showing when a narrow terminal wraps the footer", () => {
    expect(menuFooterShowing(["  2. [ ] Type something", "     Next", "Enter to select · Tab/Arrow", "keys to navigate · Esc to", "cancel"])).toBe(true);
  });

  it("sees a menu the card reader does not take", () => {
    const unknown = ["Pick a model", "", "  haiku", "  sonnet", "", "Enter to confirm · Esc to cancel"];
    expect(detectChoice(unknown)).toBeNull();
    expect(menuFooterShowing(unknown)).toBe(true);
  });
});
