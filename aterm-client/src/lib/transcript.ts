// A seat's transcript, read by the daemon from its harness's files (COI-3532).
// The client only reads it. See the transcript reference in the aterm-client skill.

export type Level = "focus" | "default" | "verbose";
export const LEVELS: readonly Level[] = ["focus", "default", "verbose"];

export type ToolClass = "shell" | "edit" | "read" | "search" | "mcp" | "other";
const TOOL_CLASSES: readonly string[] = ["shell", "edit", "read", "search", "mcp", "other"];

export interface ToolStep {
  name: string;
  class: ToolClass;
  /** One line from the daemon, with an edit's counts. Focus shows only this. */
  summary: string;
  /** The whole result, which the client cuts for Default. */
  output?: string;
  ok?: boolean;
  outputBytes?: number;
  /** The daemon cut the output at its own cap. */
  truncated?: boolean;
  /** An edit's counts. The patch text is never sent. */
  diff?: { files: number; added: number; removed: number };
}

export interface TranscriptItem {
  id: string;
  /** A prompt is the person's turn. A peer is another seat's message. */
  kind: "prompt" | "peer" | "reply" | "tool" | "note";
  text?: string;
  /** Epoch milliseconds, when the harness wrote it. */
  at?: number;
  model?: string;
  /** For a reply: the turn ended on it. */
  complete?: boolean;
  /** The host cut the text at its own cap. */
  truncated?: boolean;
  /** For a peer item: who sent it. */
  from?: { role: string; identity: string };
  /** The peer item this one answers, when the daemon can tell. */
  for?: string;
  /** A peer landed mid-turn, so this may also continue the person's own task. */
  shared?: boolean;
  tool?: ToolStep;
}

export interface TranscriptReply {
  session: string;
  reader: string;
  items: TranscriptItem[];
  /** Opaque. Sent back to ask for what was written since. */
  cursor: string;
  /** Sent back as `before` to ask for the page before the first item here. */
  older?: string;
  /** Older items exist, before the first one here. */
  more: boolean;
  updatedAt?: number;
}

/** Why a seat has no transcript, in the words the view says. */
export const REFUSALS: Record<string, string> = {
  no_transcript: "This seat keeps no transcript Treehouse can read.",
  ambiguous: "More than one seat shares this seat's folder, so its transcript cannot be told apart.",
  unreadable_format: "This seat's transcript is in a format Treehouse does not read yet.",
  unsupported_harness: "This seat's program keeps no transcript Treehouse reads.",
  passkey_required: "This device must be unlocked before it can read a transcript. It can read the screen.",
  bad_cursor: "The transcript moved on while it was being read.",
};

/** The host refused a transcript read, with the reason it named. */
export class TranscriptRefused extends Error {
  constructor(readonly reason: string) {
    super(refusalText(reason));
  }
}

export function refusalText(reason: string | undefined): string {
  return (reason && REFUSALS[reason]) || "The host would not send this seat's transcript.";
}

const str = (value: unknown): string | undefined => (typeof value === "string" && value !== "" ? value : undefined);
const num = (value: unknown): number | undefined => (typeof value === "number" && Number.isFinite(value) ? value : undefined);
const stamp = (value: unknown): number | undefined => {
  const at = typeof value === "string" ? Date.parse(value) : NaN;
  return Number.isFinite(at) ? at : undefined;
};

/** One item off the wire. Anything that is not an item is dropped. */
export function decodeItem(raw: unknown): TranscriptItem | null {
  if (typeof raw !== "object" || raw === null) return null;
  const item = raw as Record<string, unknown>;
  const id = str(item.id);
  const kind = item.kind;
  if (!id || (kind !== "prompt" && kind !== "peer" && kind !== "reply" && kind !== "tool" && kind !== "note")) return null;
  const tool = typeof item.tool === "object" && item.tool !== null ? (item.tool as Record<string, unknown>) : undefined;
  const name = tool && str(tool.name);
  const diff = tool && typeof tool.diff === "object" && tool.diff !== null ? (tool.diff as Record<string, unknown>) : undefined;
  const from = typeof item.from === "object" && item.from !== null ? (item.from as Record<string, unknown>) : undefined;
  return {
    id,
    kind,
    ...(str(item.text) ? { text: str(item.text) } : {}),
    ...(stamp(item.at) !== undefined ? { at: stamp(item.at) } : {}),
    ...(str(item.model) ? { model: str(item.model) } : {}),
    ...(typeof item.complete === "boolean" ? { complete: item.complete } : {}),
    ...(item.truncated === true ? { truncated: true } : {}),
    ...(str(item.for) ? { for: str(item.for) } : {}),
    ...(item.shared === true ? { shared: true } : {}),
    ...(from && str(from.role) && str(from.identity) ? { from: { role: str(from.role) as string, identity: str(from.identity) as string } } : {}),
    ...(tool && name
      ? {
          tool: {
            name,
            class: TOOL_CLASSES.includes(String(tool.class)) ? (tool.class as ToolClass) : "other",
            summary: str(tool.summary) ?? name,
            ...(typeof tool.output === "string" ? { output: tool.output } : {}),
            ...(typeof tool.ok === "boolean" ? { ok: tool.ok } : {}),
            ...(num(tool.output_bytes) !== undefined ? { outputBytes: num(tool.output_bytes) } : {}),
            ...(tool.truncated === true ? { truncated: true } : {}),
            ...(diff && num(diff.files) !== undefined && num(diff.added) !== undefined && num(diff.removed) !== undefined
              ? { diff: { files: num(diff.files) as number, added: num(diff.added) as number, removed: num(diff.removed) as number } }
              : {}),
          },
        }
      : {}),
  };
}

export function decodeReply(session: string, frame: Record<string, unknown>): TranscriptReply {
  const items = Array.isArray(frame.items) ? frame.items.map(decodeItem).filter((item): item is TranscriptItem => item !== null) : [];
  return {
    session,
    reader: str(frame.reader) ?? "",
    items,
    cursor: str(frame.cursor) ?? "",
    ...(str(frame.older) ? { older: str(frame.older) } : {}),
    more: frame.more === true,
    ...(stamp(frame.updated_at) !== undefined ? { updatedAt: stamp(frame.updated_at) } : {}),
  };
}

/** Joins items by id, so an item that changed replaces in place. */
export function mergeItems(held: readonly TranscriptItem[], next: readonly TranscriptItem[]): TranscriptItem[] {
  const out = [...held];
  for (const item of next) {
    const at = out.findIndex((each) => each.id === item.id);
    if (at === -1) out.push(item);
    else out[at] = item;
  }
  return out;
}

// A peer message and the replies tagged to it leave the view. A tagged item the daemon
// calls shared may also be the person's own work, so it stays and says so (Kai decides).
export function withoutPeers(items: readonly TranscriptItem[]): TranscriptItem[] {
  return items.filter((item) => item.kind !== "peer" && (item.for === undefined || item.shared === true));
}

/** A row the view draws. */
export type Row =
  | { key: string; kind: "prompt" | "reply" | "note"; text: string; at?: number; model?: string; maybePeer: boolean; cut: boolean }
  | { key: string; kind: "tool"; name: string; summary: string; lines: string[]; hidden: number; ok: boolean | undefined; cut: boolean; maybePeer: boolean };

/** Lines of a result Default shows before saying how many more there are. */
export const DEFAULT_OUTPUT_LINES = 3;

// Claude Code's viewMode: Focus is a turn's prompt, a line per tool and its final reply.
// Default keeps every reply and a few lines of each result. Verbose keeps everything.
export function rowsFor(all: readonly TranscriptItem[], level: Level): Row[] {
  const items = withoutPeers(all);
  const rows: Row[] = [];
  const lastReply = new Map<number, string>();
  let turn = 0;
  items.forEach((item) => {
    if (item.kind === "prompt") turn += 1;
    else if (item.kind === "reply") lastReply.set(turn, item.id);
  });
  turn = 0;
  for (const item of items) {
    if (item.kind === "prompt") {
      turn += 1;
      if (item.text) rows.push({ key: item.id, kind: "prompt", text: item.text, maybePeer: item.shared === true, cut: item.truncated === true, ...(item.at ? { at: item.at } : {}) });
    } else if (item.kind === "reply") {
      if (!item.text) continue;
      if (level === "focus" && lastReply.get(turn) !== item.id) continue;
      rows.push({ key: item.id, kind: "reply", text: item.text, maybePeer: item.shared === true, cut: item.truncated === true, ...(item.at ? { at: item.at } : {}), ...(item.model ? { model: item.model } : {}) });
    } else if (item.kind === "tool" && item.tool) {
      const all = level === "focus" || !item.tool.output ? [] : item.tool.output.replace(/\n+$/, "").split("\n");
      const shown = level === "verbose" ? all : all.slice(0, DEFAULT_OUTPUT_LINES);
      rows.push({
        key: item.id,
        kind: "tool",
        name: item.tool.name,
        summary: item.tool.summary,
        lines: shown,
        hidden: all.length - shown.length,
        ok: item.tool.ok,
        cut: item.tool.truncated === true,
        maybePeer: item.shared === true,
      });
    } else if (item.kind === "note" && level !== "focus" && item.text) {
      rows.push({ key: item.id, kind: "note", text: item.text, maybePeer: item.shared === true, cut: item.truncated === true });
    }
  }
  return rows;
}
