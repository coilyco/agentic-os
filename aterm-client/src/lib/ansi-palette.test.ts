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
const names = ["red", "green", "yellow", "blue", "magenta", "cyan", "white"].flatMap((name) => [name, `bright-${name}`]).concat("bright-black", "black");
const light = css.slice(css.indexOf(':root[data-theme="light"]'));

// The colour a harness's background maps to draws no rectangle, so only it may go unread.
const themes = [
  { name: "dark", tokens: css, ground: "black" },
  { name: "light", tokens: light, ground: "white" },
];

describe.each(themes)("the terminal's ANSI palette on the $name --terminal ground (COI-3552, COI-3554)", ({ tokens, ground: mapped }) => {
  const ground = tokenHex(tokens, "--terminal");
  it.each(names.filter((name) => name !== mapped))("%s reads at 4.5:1 or better", (name) => {
    expect(contrast(tokenHex(tokens, `--ansi-${name}`), ground), name).toBeGreaterThanOrEqual(4.5);
  });
  it(`paints ${mapped} as the ground, so a TUI's ${mapped} background draws no rectangle`, () => {
    expect(tokenHex(tokens, `--ansi-${mapped}`)).toBe(ground);
  });
});

// The words and signals on every step they sit on (COI-3549, COI-3554).
const texts = ["--text", "--text-soft", "--muted", "--treehouse"];
const grounds = ["--ground", "--terminal", "--surface", "--hover", "--raised", "--selected"];
describe.each(themes)("the $name theme's text tokens", ({ tokens }) => {
  it.each(texts.flatMap((text) => grounds.map((ground) => [text, ground])))("%s reads on %s at 4.5:1", (text, ground) => {
    expect(contrast(tokenHex(tokens, text!), tokenHex(tokens, ground!)), `${text} on ${ground}`).toBeGreaterThanOrEqual(4.5);
  });
  it("reads the button's ink on its green at 4.5:1", () => {
    expect(contrast(tokenHex(tokens, "--brand-ink"), tokenHex(tokens, "--treehouse"))).toBeGreaterThanOrEqual(4.5);
  });
  it("reads amber and red text on the page and on their own fills", () => {
    for (const [text, fill] of [["--warn-text", "--warn-fill"], ["--danger-text", "--danger-fill"], ["--danger-detail", "--danger-fill"]]) {
      expect(contrast(tokenHex(tokens, text!), tokenHex(tokens, fill!)), `${text} on ${fill}`).toBeGreaterThanOrEqual(4.5);
      expect(contrast(tokenHex(tokens, text!), tokenHex(tokens, "--ground")), `${text} on ground`).toBeGreaterThanOrEqual(4.5);
    }
  });
});
