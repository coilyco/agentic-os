import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { tokenHex } from "../../pwa-plugin.ts";

const css = readFileSync(new URL("../app.css", import.meta.url), "utf8");
const channel = (c: number) => (c / 255 <= 0.03928 ? c / 255 / 12.92 : ((c / 255 + 0.055) / 1.055) ** 2.4);
const luminance = (hex: string) => {
  const [r, g, b] = [1, 3, 5].map((at) => channel(parseInt(hex.slice(at, at + 2), 16)));
  return 0.2126 * r! + 0.7152 * g! + 0.0722 * b!;
};
const contrast = (a: string, b: string) => {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi! + 0.05) / (lo! + 0.05);
};
const names = ["red", "green", "yellow", "blue", "magenta", "cyan", "white"].flatMap((name) => [name, `bright-${name}`]).concat("bright-black");

describe("the terminal's ANSI palette on the --terminal ground (COI-3552)", () => {
  const ground = tokenHex(css, "--terminal");
  it.each(names)("%s reads at 4.5:1 or better", (name) => {
    expect(contrast(tokenHex(css, `--ansi-${name}`), ground), name).toBeGreaterThanOrEqual(4.5);
  });
  it("paints black as the ground, so a TUI's black background draws no rectangle", () => {
    expect(tokenHex(css, "--ansi-black")).toBe(ground);
  });
});
