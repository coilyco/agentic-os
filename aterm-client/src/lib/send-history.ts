// What a person sent to a seat, kept on the device and walked with arrows (COI-3691).
// Recorded at the press, before delivery, so a send a dialog swallowed is still here.
const KEY = "aterm.send-history.v1";

export const KEEP_MS = 30 * 24 * 60 * 60 * 1000;
export const PER_SEAT = 50;
export const MAX_SEATS = 30;
/** A longer send is not kept, so storage is never the reason a send fails. */
export const MAX_CHARS = 20000;

export interface Entry {
  text: string;
  /** When Send was pressed, in ms. */
  at: number;
  /** How many images went with it. Their bytes are in history-images.ts under `id`. */
  images?: number;
  id?: string;
  /** The images' names and sizes, so a press repeated while Send is held is one entry. */
  sig?: string;
}

/** The images of a send, as the Composer holds them. */
export function describeImages(files: readonly { name: string; size: number }[]): { count: number; sig: string } {
  return { count: files.length, sig: files.map((file) => `${file.name}:${file.size}`).join(",") };
}

/** Image sends kept with their images per seat. Older ones keep their words only. */
export const IMAGE_SENDS = 10;

/** Oldest first within a seat, by host and session. */
export type Stored = Record<string, Entry[]>;

export function parseHistory(text: string | null, now: number): Stored {
  try {
    const raw: unknown = JSON.parse(text ?? "null");
    if (typeof raw !== "object" || raw === null) return {};
    const kept: Stored = {};
    for (const [key, list] of Object.entries(raw)) {
      if (!Array.isArray(list)) continue;
      const entries = list.filter((one): one is Entry => typeof one?.text === "string" && typeof one?.at === "number" && now - one.at <= KEEP_MS && (one.text !== "" || (typeof one.images === "number" && one.images > 0)));
      if (entries.length) kept[key] = entries.slice(-PER_SEAT);
    }
    return kept;
  } catch {
    return {};
  }
}

/** The store after a send. The same words and images twice in a row are one entry. */
export function withSend(stored: Stored, key: string, text: string, now: number, attached?: { count: number; sig: string; id: string }): Stored {
  if ((!text.trim() && !attached?.count) || text.length > MAX_CHARS) return stored;
  const list = (stored[key] ?? []).filter((one) => now - one.at <= KEEP_MS);
  let id = attached?.id;
  const last = list.at(-1);
  if (last && last.text === text && (last.sig ?? "") === (attached?.sig ?? "")) {
    id = last.id ?? id;
    list.pop();
  }
  const entry: Entry = { text, at: now, ...(attached?.count && id ? { images: attached.count, id, sig: attached.sig } : {}) };
  const kept = [...list, entry].slice(-PER_SEAT);
  // Past the newest few image sends an entry keeps its words, and one with no words goes.
  let withImages = 0;
  const trimmed: Entry[] = [];
  for (const one of [...kept].reverse()) {
    if (one.images && ++withImages > IMAGE_SENDS) {
      if (!one.text) continue;
      trimmed.push({ text: one.text, at: one.at });
    } else trimmed.push(one);
  }
  const next: Stored = { ...stored, [key]: trimmed.reverse() };
  const seats = Object.keys(next).sort((a, b) => (next[b]?.at(-1)?.at ?? 0) - (next[a]?.at(-1)?.at ?? 0));
  for (const old of seats.slice(MAX_SEATS)) delete next[old];
  return next;
}

// Held in memory too, so a device whose storage throws still walks this page's sends.
let memory: Stored | undefined;

function read(now: number): Stored {
  if (memory) return memory;
  try {
    memory = parseHistory(localStorage.getItem(KEY), now);
  } catch {
    memory = {};
  }
  return memory;
}

function write(next: Stored): void {
  memory = next;
  try {
    if (Object.keys(next).length) localStorage.setItem(KEY, JSON.stringify(next));
    else localStorage.removeItem(KEY);
  } catch {
    // Storage is full or off. The sends stay in memory.
  }
}

/** This seat's sends, newest first. */
export function recentSends(key: string, now = Date.now()): Entry[] {
  return [...(read(now)[key] ?? [])].reverse();
}

/** Records a send. `fresh` means new images, for the caller to keep under `id`. */
export function recordSend(key: string, text: string, files: readonly File[] = [], now = Date.now()): { id?: string; fresh: boolean } {
  const { count, sig } = describeImages(files);
  const last = read(now)[key]?.at(-1);
  const same = count > 0 && last !== undefined && last.text === text && last.sig === sig && last.id !== undefined;
  const id = count > 0 ? (same ? last.id : crypto.randomUUID()) : undefined;
  write(withSend(read(now), key, text, now, id ? { count, sig, id } : undefined));
  return { ...(id ? { id } : {}), fresh: count > 0 && !same };
}

export function clearSends(key: string): void {
  const next = { ...read(Date.now()) };
  delete next[key];
  write(next);
}

/** Forget what was read, so the next read starts from storage. For tests. */
export function resetHistoryMemory(): void {
  memory = undefined;
}

// The walk. `at` indexes the newest-first list, and -1 is the person's own draft.
export interface Walk {
  at: number;
  draft: string;
}

export const NOT_WALKING: Walk = { at: -1, draft: "" };

/** Up: the next older send. The draft is set aside on step one. Null at the oldest. */
export function older(entries: readonly Entry[], walk: Walk, current: string): { text: string; walk: Walk; entry?: Entry } | null {
  const at = walk.at + 1;
  const entry = entries[at];
  if (!entry) return null;
  return { text: entry.text, entry, walk: { at, draft: walk.at === -1 ? current : walk.draft } };
}

/** Down: the next newer send, then the draft. Null when not walking. */
export function newer(entries: readonly Entry[], walk: Walk): { text: string; walk: Walk; entry?: Entry } | null {
  if (walk.at === -1) return null;
  if (walk.at === 0) return { text: walk.draft, walk: NOT_WALKING };
  const entry = entries[walk.at - 1];
  return entry ? { text: entry.text, entry, walk: { at: walk.at - 1, draft: walk.draft } } : null;
}

/** Any entry by position, for a tap in the list. */
export function jumpTo(entries: readonly Entry[], walk: Walk, current: string, at: number): { text: string; walk: Walk; entry?: Entry } | null {
  const entry = entries[at];
  return entry ? { text: entry.text, entry, walk: { at, draft: walk.at === -1 ? current : walk.draft } } : null;
}

/** Up recalls from the first line. A wrapped line moves the caret first, as usual. */
export function onFirstLine(text: string, caret: number, wrapped: boolean): boolean {
  return !text.slice(0, caret).includes("\n") && (!wrapped || caret === 0);
}

export function onLastLine(text: string, caret: number, wrapped: boolean): boolean {
  return !text.slice(caret).includes("\n") && (!wrapped || caret === text.length);
}
