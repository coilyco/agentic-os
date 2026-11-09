// A seat's last reply as formatted text (COI-3543). It parses to a tree the components
// draw node by node, so no HTML string is built and raw tags stay text.

export type Inline =
  | { t: "text"; v: string }
  | { t: "code"; v: string }
  | { t: "strong" | "em" | "del"; c: Inline[] }
  | { t: "link"; href: string; c: Inline[] };

export type Block =
  | { t: "p"; c: Inline[] }
  | { t: "h"; level: number; c: Inline[] }
  | { t: "code"; v: string }
  | { t: "quote"; c: Block[] }
  | { t: "list"; ordered: boolean; start: number; items: Block[][] }
  | { t: "table"; head: Inline[][]; rows: Inline[][][] }
  | { t: "hr" };

const SAFE = /^(https?:\/\/|mailto:)\S+$/i;
const BARE = /https?:\/\/[^\s<>]+/g;

/** The address when it is one a person may open, otherwise null (no `javascript:`). */
export function safeHref(url: string): string | null {
  const clean = url.trim().replace(/^<|>$/g, "");
  return SAFE.test(clean) ? clean : null;
}

function withBareLinks(text: string, out: Inline[]): void {
  let last = 0;
  for (const found of text.matchAll(BARE)) {
    const url = found[0].replace(/[.,;:!?'")\]]+$/, "");
    const at = found.index ?? 0;
    if (at > last) out.push({ t: "text", v: text.slice(last, at) });
    out.push({ t: "link", href: url, c: [{ t: "text", v: url }] });
    last = at + url.length;
  }
  if (last < text.length) out.push({ t: "text", v: text.slice(last) });
}

/** The index of the `]` closing the `[` at `from`, or -1. Brackets nest. */
function closeBracket(src: string, from: number): number {
  let depth = 0;
  for (let i = from; i < src.length; i++) {
    if (src[i] === "\\") i++;
    else if (src[i] === "[") depth++;
    else if (src[i] === "]" && --depth === 0) return i;
  }
  return -1;
}

function closeParen(src: string, from: number): number {
  let depth = 0;
  for (let i = from; i < src.length; i++) {
    if (src[i] === "\\") i++;
    else if (src[i] === "(") depth++;
    else if (src[i] === ")" && --depth === 0) return i;
  }
  return -1;
}

/** Where the emphasis opened at `from` closes, or -1. Skips code spans. */
function closer(src: string, from: number, mark: string): number {
  for (let i = from; i < src.length; i++) {
    const ch = src[i];
    if (ch === "\\") i++;
    else if (ch === "`") {
      const run = /^`+/.exec(src.slice(i))![0];
      const end = src.indexOf(run, i + run.length);
      i = end < 0 ? src.length : end + run.length - 1;
    } else if (src.startsWith(mark, i) && i > from && !/\s/.test(src[i - 1]!)) {
      const after = src[i + mark.length] ?? "";
      if (mark.length === 1 && after === mark) i++;
      else if (mark[0] === "_" && /[\p{L}\p{N}]/u.test(after)) continue;
      else return i;
    }
  }
  return -1;
}

const ESCAPABLE = /[\\`*_{}[\]()#+\-.!~>|<]/;

// `cut`: the text may end mid-way, so an opener with no closer formats to the end.
export function parseInline(src: string, cut = false): Inline[] {
  const out: Inline[] = [];
  let text = "";
  const flush = (): void => {
    if (text) withBareLinks(text, out);
    text = "";
  };
  let i = 0;
  while (i < src.length) {
    const ch = src[i]!;
    if (ch === "\\" && ESCAPABLE.test(src[i + 1] ?? "")) {
      text += src[i + 1];
      i += 2;
    } else if (ch === "`") {
      const run = /^`+/.exec(src.slice(i))![0];
      const end = src.indexOf(run, i + run.length);
      if (end > 0 || cut) {
        flush();
        const v = src.slice(i + run.length, end > 0 ? end : src.length).trim();
        if (v) out.push({ t: "code", v });
        i = end > 0 ? end + run.length : src.length;
      } else {
        text += run;
        i += run.length;
      }
    } else if (ch === "[") {
      const close = closeBracket(src, i);
      const label = close < 0 ? src.slice(i + 1) : src.slice(i + 1, close);
      if (close < 0) {
        if (!cut) {
          text += ch;
          i++;
          continue;
        }
        flush();
        out.push(...parseInline(label, cut));
        i = src.length;
      } else if (src[close + 1] === "(") {
        const end = closeParen(src, close + 1);
        const url = src.slice(close + 2, end < 0 ? src.length : end);
        if (end < 0 && !cut) {
          text += ch;
          i++;
          continue;
        }
        flush();
        const href = end < 0 ? null : safeHref(url.split(/\s+"/)[0]!);
        if (href) out.push({ t: "link", href, c: parseInline(label) });
        else out.push(...parseInline(label));
        i =end < 0 ? src.length : end + 1;
      } else {
        text += src.slice(i, close + 1);
        i = close + 1;
      }
    } else if (ch === "*" || ch === "_" || ch === "~") {
      const double = src[i + 1] === ch;
      const mark = ch === "~" ? (double ? "~~" : "") : double ? ch + ch : ch;
      const before = src[i - 1] ?? "";
      const next = src[i + mark.length] ?? "";
      const opens = mark !== "" && next !== "" && !/\s/.test(next) && !(ch === "_" && /[\p{L}\p{N}]/u.test(before));
      if (!opens) {
        text += mark || ch;
        i += (mark || ch).length;
        continue;
      }
      const end = closer(src, i + mark.length, mark);
      if (end < 0 && !cut) {
        text += mark;
        i += mark.length;
        continue;
      }
      flush();
      const inner = src.slice(i + mark.length, end < 0 ? src.length : end);
      out.push({ t: mark === "~~" ? "del" : mark.length === 2 ? "strong" : "em", c: parseInline(inner, cut) });
      i = end < 0 ? src.length : end + mark.length;
    } else {
      text += ch;
      i++;
    }
  }
  flush();
  return out;
}

const FENCE = /^ {0,3}(`{3,}|~{3,})\s*[^`]*$/;
const HEADING = /^ {0,3}(#{1,6})\s+(.*?)(?:\s+#+)?\s*$/;
const RULE = /^ {0,3}([-*_])(?:\s*\1){2,}\s*$/;
const QUOTE = /^ {0,3}>\s?/;
const ITEM = /^( {0,3})([-*+]|\d{1,9}[.)])\s+(.*)$/;
const TABLE_RULE = /^\s*\|?\s*:?-{1,}:?\s*(\|\s*:?-{1,}:?\s*)*\|?\s*$/;

const cells = (line: string): string[] => line.trim().replace(/^\||\|$/g, "").split(/(?<!\\)\|/).map((cell) => cell.trim().replace(/\\\|/g, "|"));

const startsBlock = (line: string): boolean => FENCE.test(line) || HEADING.test(line) || RULE.test(line) || QUOTE.test(line) || ITEM.test(line);

/** Block structure of a whole reply. Text that is no known block is a paragraph. */
export function parseBlocks(src: string): Block[] {
  const lines = src.replace(/\r\n?/g, "\n").split("\n");
  const blocks: Block[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i]!;
    if (!line.trim()) {
      i++;
      continue;
    }
    const fence = FENCE.exec(line);
    if (fence) {
      const mark = fence[1]!;
      const body: string[] = [];
      for (i++; i < lines.length && !new RegExp(`^ {0,3}${mark[0] === "`" ? "`" : "~"}{${mark.length},}\\s*$`).test(lines[i]!); i++) body.push(lines[i]!);
      i++;
      blocks.push({ t: "code", v: body.join("\n") });
      continue;
    }
    const heading = HEADING.exec(line);
    if (heading) {
      blocks.push({ t: "h", level: heading[1]!.length, c: parseInline(heading[2]!) });
      i++;
      continue;
    }
    if (RULE.test(line)) {
      blocks.push({ t: "hr" });
      i++;
      continue;
    }
    if (QUOTE.test(line)) {
      const body: string[] = [];
      for (; i < lines.length && QUOTE.test(lines[i]!); i++) body.push(lines[i]!.replace(QUOTE, ""));
      blocks.push({ t: "quote", c: parseBlocks(body.join("\n")) });
      continue;
    }
    const item = ITEM.exec(line);
    if (item) {
      const ordered = /\d/.test(item[2]![0]!);
      const items: Block[][] = [];
      const start = ordered ? parseInt(item[2]!, 10) : 1;
      while (i < lines.length) {
        const head = ITEM.exec(lines[i]!);
        if (!head || /\d/.test(head[2]![0]!) !== ordered || head[1]!.length > 3) break;
        const width = head[1]!.length + head[2]!.length + 1;
        const body = [head[3]!];
        for (i++; i < lines.length; i++) {
          const next = lines[i]!;
          if (!next.trim()) {
            const after = lines.slice(i + 1).find((each) => each.trim());
            if (after !== undefined && /^\s{2,}\S/.test(after)) {
              body.push("");
              continue;
            }
            break;
          }
          if (/^\s+\S/.test(next) && !(ITEM.test(next) && next.search(/\S/) < width)) body.push(next.slice(Math.min(width, next.search(/\S/))));
          else if (!startsBlock(next) && body[body.length - 1]!.trim()) body.push(next);
          else break;
        }
        items.push(parseBlocks(body.join("\n")));
        while (i < lines.length && !lines[i]!.trim() && ITEM.test(lines.slice(i + 1).find((each) => each.trim()) ?? "")) i++;
      }
      blocks.push({ t: "list", ordered, start, items });
      continue;
    }
    if (line.includes("|") && i + 1 < lines.length && TABLE_RULE.test(lines[i + 1]!) && lines[i + 1]!.includes("-")) {
      const head = cells(line).map((cell) => parseInline(cell));
      const rows: Inline[][][] = [];
      for (i += 2; i < lines.length && lines[i]!.trim() && lines[i]!.includes("|"); i++) rows.push(cells(lines[i]!).map((cell) => parseInline(cell)));
      blocks.push({ t: "table", head, rows });
      continue;
    }
    const body: string[] = [];
    for (; i < lines.length && lines[i]!.trim() && (body.length === 0 || !startsBlock(lines[i]!)); i++) body.push(lines[i]!.trim());
    blocks.push({ t: "p", c: parseInline(body.join("\n")) });
  }
  return blocks;
}

/** The inline words of a one-line view: the pinned line and the sidebar preview. */
export function parseLine(first: string): Inline[] {
  const bare = first.replace(/^\s*(?:#{1,6}\s+|>\s*|(?:[-*+]|\d{1,9}[.)])\s+)/, "");
  return parseInline(bare, true);
}

/** What a node reads as with the marks gone, for a label read aloud or a cut preview. */
export function plainText(nodes: readonly Inline[]): string {
  return nodes.map((node) => (node.t === "text" || node.t === "code" ? node.v : plainText(node.c))).join("");
}
