import { describe, expect, it } from "vitest";
import { parseBlocks, parseInline, parseLine, plainText, safeHref } from "./markdown";

const text = (v: string) => ({ t: "text", v });

describe("inline", () => {
  it("formats bold, italic, code and strike", () => {
    expect(parseInline("a **b** *c* `d` ~~e~~")).toEqual([
      text("a "), { t: "strong", c: [text("b")] }, text(" "), { t: "em", c: [text("c")] }, text(" "), { t: "code", v: "d" }, text(" "), { t: "del", c: [text("e")] },
    ]);
  });
  it("leaves an unclosed mark as the person typed it when the text is whole", () => {
    expect(plainText(parseInline("2 * 3 and **open"))).toBe("2 * 3 and **open");
  });
  it("formats an unclosed mark to the end when the text was cut short", () => {
    expect(plainText(parseInline("it did **not pass the", true))).toBe("it did not pass the");
    expect(parseInline("run `just aterm-cl", true).at(-1)).toEqual({ t: "code", v: "just aterm-cl" });
  });
  it("does not read snake_case as italic", () => {
    expect(parseInline("last_reply_at and other_thing")).toEqual([text("last_reply_at and other_thing")]);
  });
  it("makes a link from a label and an address", () => {
    expect(parseInline("see [PR 304](https://github.com/x/y/pull/304).")).toEqual([
      text("see "), { t: "link", href: "https://github.com/x/y/pull/304", c: [text("PR 304")] }, text("."),
    ]);
  });
  it("links a bare address and leaves its closing punctuation out", () => {
    expect(parseInline("at https://example.com/a, ok")).toEqual([
      text("at "), { t: "link", href: "https://example.com/a", c: [text("https://example.com/a")] }, text(", ok"),
    ]);
  });
  it("shows only the label when the address is not a web one", () => {
    const nodes = parseInline("[click](javascript:alert(1))");
    expect(nodes).toEqual([text("click")]);
  });
  it("drops the half of a link that was cut", () => {
    expect(plainText(parseInline("read [the notes](https://exam", true))).toBe("read the notes");
    expect(plainText(parseInline("read [the not", true))).toBe("read the not");
  });
  it("keeps raw HTML as text", () => {
    expect(parseInline("<img src=x onerror=alert(1)> <b>hi</b>")).toEqual([text("<img src=x onerror=alert(1)> <b>hi</b>")]);
  });
  it("honours a backslash escape", () => {
    expect(plainText(parseInline("\\*not bold\\*"))).toBe("*not bold*");
  });
});

describe("blocks", () => {
  it("reads headings, rules and paragraphs", () => {
    expect(parseBlocks("## Done\n\nIt works.\n\n---\n")).toEqual([
      { t: "h", level: 2, c: [text("Done")] }, { t: "p", c: [text("It works.")] }, { t: "hr" },
    ]);
  });
  it("reads a fenced block as it was written, markup included", () => {
    expect(parseBlocks("```ts\nconst a = **b**;\n```\nafter")).toEqual([
      { t: "code", v: "const a = **b**;" }, { t: "p", c: [text("after")] },
    ]);
  });
  it("reads an unclosed fence to the end", () => {
    expect(parseBlocks("```\nrunning")).toEqual([{ t: "code", v: "running" }]);
  });
  it("reads bullet and numbered lists", () => {
    const [bullets, numbered] = parseBlocks("- a\n- **b**\n\n3. c\n4. d");
    expect(bullets).toMatchObject({ t: "list", ordered: false });
    expect((bullets as { items: unknown[] }).items).toHaveLength(2);
    expect(numbered).toMatchObject({ t: "list", ordered: true, start: 3 });
  });
  it("nests a list under an item", () => {
    const [list] = parseBlocks("- a\n  - b\n  - c\n- d");
    const items = (list as { items: { t: string }[][] }).items;
    expect(items).toHaveLength(2);
    expect(items[0]!.map((block) => block.t)).toEqual(["p", "list"]);
  });
  it("reads a quote and its content", () => {
    expect(parseBlocks("> **note**\n> more")).toMatchObject([{ t: "quote", c: [{ t: "p" }] }]);
  });
  it("reads a table", () => {
    const [table] = parseBlocks("| a | b |\n|---|---|\n| 1 | `2` |");
    expect(table).toMatchObject({ t: "table" });
    expect((table as { rows: unknown[] }).rows).toHaveLength(1);
  });
  it("keeps a line of raw HTML as a paragraph of text", () => {
    expect(parseBlocks("<script>alert(1)</script>")).toEqual([{ t: "p", c: [text("<script>alert(1)</script>")] }]);
  });
});

describe("the one-line view", () => {
  it("drops a leading list or heading mark", () => {
    expect(plainText(parseLine("- **Fixed** the `close` bar"))).toBe("Fixed the close bar");
    expect(plainText(parseLine("## Summary of it"))).toBe("Summary of it");
  });
  it("shows no stray mark when the daemon cut the line mid-word", () => {
    expect(plainText(parseLine("Merged **PR 304 and the `orig"))).toBe("Merged PR 304 and the orig");
  });
});

describe("safeHref", () => {
  it("allows web and mail addresses only", () => {
    expect(safeHref("https://a.b/c")).toBe("https://a.b/c");
    expect(safeHref("mailto:a@b.c")).toBe("mailto:a@b.c");
    expect(safeHref("javascript:alert(1)")).toBeNull();
    expect(safeHref("data:text/html,x")).toBeNull();
    expect(safeHref("/relative")).toBeNull();
  });
});
