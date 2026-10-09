import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";

// Nothing moves the selected seat without a click or tap. This scan fails when a file
// outside the click handlers starts selecting a seat.
const SRC = join(__dirname, "..");

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return name === "fixtures" || name === "assets" ? [] : sources(path);
    return /\.(ts|svelte)$/.test(name) && !/\.test\.ts$/.test(name) ? [path] : [];
  });
}

const files = sources(SRC).map((path) => ({ name: relative(SRC, path), text: readFileSync(path, "utf8") }));
const filesWith = (needle: RegExp) => files.filter((file) => needle.test(file.text)).map((file) => file.name).sort();

describe("seat selection", () => {
  it("is done only by the store and by components that run it from a click or tap", () => {
    expect(filesWith(/\b(selectSession|openByPerson)\(/)).toEqual([
      "components/SeatToast.svelte",
      "components/Sidebar.svelte",
      "lib/app.svelte.ts",
      "panels/LaunchPanel.svelte",
    ]);
  });

  it("is written only inside the store, so no component sets the selected seat on its own", () => {
    expect(filesWith(/app\.selectedSession\s*=[^=]/)).toEqual(["lib/app.svelte.ts"]);
  });

  it("has no alt-tab jump, no triage walk, and no timer that opens a seat", () => {
    for (const word of ["jumpToWaiting", "worthJumping", "nextIfTriaging", "app.triage"]) expect(filesWith(new RegExp(word.replace(".", "\\.")))).toEqual([]);
    for (const file of ["App.svelte", "panels/SessionPanel.svelte", "components/Attention.svelte"]) {
      const text = files.find((each) => each.name === file)?.text ?? "";
      expect(text, file).not.toMatch(/selectSession|openByPerson/);
    }
  });
});
