// A seat's last transcript page outlives a closed app, so reopening it paints at once
// and catches up (COI-3534). Per device, by host and session, for a day. It keeps what
// was said and a line per tool, never a tool's output.
import type { TranscriptItem } from "./transcript";

const DB_NAME = "aterm.transcripts.v1";
const STORE = "pages";

export const KEEP_MS = 24 * 60 * 60 * 1000;
export const MAX_SEATS = 12;
export const MAX_ITEMS = 60;

export interface Snapshot {
  key: string;
  at: number;
  items: TranscriptItem[];
  /** Sent back as `before` to read earlier than the first item held. */
  older?: string;
}

export const cacheKey = (hostId: string, sessionId: string): string => `${hostId}/${sessionId}`;

/** What is worth keeping: the newest items, each without the tool output it carried. */
export function trimForStore(items: readonly TranscriptItem[]): TranscriptItem[] {
  return items.slice(-MAX_ITEMS).map((item) => {
    if (!item.tool || item.tool.output === undefined) return item;
    const { output: _output, ...tool } = item.tool;
    return { ...item, tool };
  });
}

/** The keys to drop: the stale ones, then the oldest past the seat cap. */
export function planPrune(rows: Pick<Snapshot, "key" | "at">[], keep: string, now: number): string[] {
  const stale = rows.filter((row) => row.key !== keep && now - row.at > KEEP_MS).map((row) => row.key);
  const live = rows.filter((row) => !stale.includes(row.key)).sort((a, b) => a.at - b.at);
  const over = Math.max(0, live.length - MAX_SEATS);
  return [...stale, ...live.filter((row) => row.key !== keep).slice(0, over).map((row) => row.key)];
}

export interface CaughtUp {
  items: TranscriptItem[];
  older?: string;
  /** The first item the person has not seen, or undefined when nothing is new. */
  newFrom?: string;
  /** The page read was not contiguous with the held one, so the held one was dropped. */
  gap: boolean;
}

/** Joins the held page with the newest read by id. A read sharing no item drops it. */
export function catchUp(held: Snapshot, fresh: readonly TranscriptItem[], freshOlder: string | undefined): CaughtUp {
  const seen = new Set(held.items.map((item) => item.id));
  const overlap = fresh.some((item) => seen.has(item.id));
  if (!overlap && held.items.length > 0 && fresh.length > 0) return { items: [...fresh], ...(freshOlder ? { older: freshOlder } : {}), ...(fresh.length ? { newFrom: fresh[0]!.id } : {}), gap: true };
  const items = [...held.items];
  for (const item of fresh) {
    const at = items.findIndex((each) => each.id === item.id);
    if (at === -1) items.push(item);
    else items[at] = item;
  }
  const firstNew = fresh.find((item) => !seen.has(item.id));
  const reachedBack = fresh.length > 0 && !seen.has(fresh[0]!.id);
  const older = reachedBack ? freshOlder : (held.older ?? freshOlder);
  return { items, ...(older ? { older } : {}), ...(firstNew ? { newFrom: firstNew.id } : {}), gap: false };
}

/** The first drawn row at or after the first new item, where the marker goes. */
export function markerRow(items: readonly TranscriptItem[], rowKeys: readonly string[], newFrom: string | undefined): string | undefined {
  if (!newFrom) return undefined;
  const start = items.findIndex((item) => item.id === newFrom);
  if (start === -1) return undefined;
  const drawn = new Set(rowKeys);
  for (let at = start; at < items.length; at++) if (drawn.has(items[at]!.id)) return items[at]!.id;
  return undefined;
}

const wrap = <T>(request: IDBRequest<T>): Promise<T> =>
  new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });

function open(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1);
    request.onupgradeneeded = () => request.result.createObjectStore(STORE, { keyPath: "key" });
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

async function inStore<T>(mode: IDBTransactionMode, run: (store: IDBObjectStore) => Promise<T>): Promise<T> {
  const db = await open();
  try {
    return await run(db.transaction(STORE, mode).objectStore(STORE));
  } finally {
    db.close();
  }
}

/** The page kept for this seat, or null. Storage that is off or full reads as nothing. */
export async function loadSnapshot(key: string, now = Date.now()): Promise<Snapshot | null> {
  try {
    const row = await inStore("readonly", (store) => wrap(store.get(key) as IDBRequest<Snapshot | undefined>));
    return row && now - row.at <= KEEP_MS && Array.isArray(row.items) && row.items.length > 0 ? row : null;
  } catch {
    return null;
  }
}

/** Keeps this seat's page and drops what is stale or over the cap. Failure is silent. */
export async function saveSnapshot(key: string, items: readonly TranscriptItem[], older: string | undefined, now = Date.now()): Promise<void> {
  try {
    await inStore("readwrite", async (store) => {
      await wrap(store.put({ key, at: now, items: trimForStore(items), ...(older ? { older } : {}) } satisfies Snapshot));
      const rows = await wrap(store.getAll() as IDBRequest<Snapshot[]>);
      for (const drop of planPrune(rows, key, now)) await wrap(store.delete(drop));
    });
  } catch {
    // Storage can be off or full. The next open reads the page from the host as before.
  }
}
