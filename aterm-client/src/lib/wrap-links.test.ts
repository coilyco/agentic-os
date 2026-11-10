import { describe, expect, it } from "vitest";
import { lineChars, urlAt, urlAtChain } from "./wrap-links";

const LINK = "https://code.claude.com/docs/en/permission-modes";

describe("the link under a tap", () => {
  it("is the address that covers the character, and none past it", () => {
    const text = `see ${LINK} for the modes`;
    expect(urlAt(text, 4)).toBe(LINK);
    expect(urlAt(text, 4 + LINK.length - 1)).toBe(LINK);
    expect(urlAt(text, 0)).toBeNull();
    expect(urlAt(text, 4 + LINK.length + 1)).toBeNull();
  });

  it("leaves trailing punctuation out, as an unfolded row does", () => {
    expect(urlAt(`read ${LINK}.`, 10)).toBe(LINK);
    expect(urlAt(`(${LINK})`, 5)).toBe(LINK);
  });

  it("finds an address the terminal wrapped across rows from a tap on either part", () => {
    const text = `see ${LINK} now`;
    const cut = 20;
    const chain = [text.slice(0, cut), text.slice(cut)];
    expect(urlAtChain(chain, 0, 6)).toBe(LINK);
    expect(urlAtChain(chain, 1, 3)).toBe(LINK);
    expect(urlAtChain(chain, 1, chain[1]!.length - 1)).toBeNull();
  });

  it("separates two addresses on one line", () => {
    const text = "https://a.example/x and https://b.example/y";
    expect(urlAt(text, 2)).toBe("https://a.example/x");
    expect(urlAt(text, 30)).toBe("https://b.example/y");
    expect(urlAt(text, 19)).toBeNull();
  });
});

describe("a buffer row as text", () => {
  const row = (cells: [string, number][]) => ({ length: cells.length, getCell: (x: number) => ({ getChars: () => cells[x]![0], getWidth: () => cells[x]![1] }) });

  it("keeps a wide character once and an empty cell as a space", () => {
    expect(lineChars(row([["a", 1], ["", 1], ["漢", 2], ["", 0], ["b", 1]]))).toBe("a 漢b");
  });
});
