// A second copy of a draft's words in IndexedDB (COI-3619). The WebView commits local
// storage on a delay, so a force-stop soon after typing can lose words draft-store.ts
// already wrote. IndexedDB commits within milliseconds. Load takes the newer copy.
import { KEEP_MS, MAX_CHARS, MAX_DRAFTS } from "./draft-store";

const DB_NAME = "aterm.draft-words.v1";
const STORE = "words";

export interface Row {
  key: string;
  text: string;
  at: number;
}

/** The keys to drop: the stale ones, then the oldest past the cap. */
export function planWordPrune(rows: Pick<Row, "key" | "at">[], now: number): string[] {
  const drop = rows.filter((row) => now - row.at > KEEP_MS).map((row) => row.key);
  const live = rows.filter((row) => !drop.includes(row.key)).sort((a, b) => b.at - a.at);
  return [...drop, ...live.slice(MAX_DRAFTS).map((row) => row.key)];
}

/** The mirror's words when it is newer than the stored copy, else nothing. */
export function newerWords(stored: { text: string; at: number } | undefined, mirrored: Row | undefined): string | undefined {
  if (!mirrored?.text) return undefined;
  return !stored || mirrored.at > stored.at ? mirrored.text : undefined;
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

// One write at a time, so an earlier put never lands over a later one.
let queue: Promise<unknown> = Promise.resolve();

/** Mirror a seat's words. Empty or over the cap removes the copy, as draft-store does. */
export function mirrorDraft(key: string, text: string, now = Date.now()): Promise<void> {
  const write = async () => {
    try {
      const db = await open();
      try {
        const store = db.transaction(STORE, "readwrite").objectStore(STORE);
        if (!text || text.length > MAX_CHARS) await wrap(store.delete(key));
        else await wrap(store.put({ key, text, at: now } satisfies Row));
        for (const old of planWordPrune((await wrap(store.getAll())) as Row[], now)) await wrap(store.delete(old));
      } finally {
        db.close();
      }
    } catch {
      // IndexedDB can be off (a private window). Local storage still holds the words.
    }
  };
  queue = queue.then(write, write);
  return queue as Promise<void>;
}

export async function loadMirroredDraft(key: string, now = Date.now()): Promise<Row | undefined> {
  try {
    const db = await open();
    try {
      const row = (await wrap(db.transaction(STORE, "readonly").objectStore(STORE).get(key))) as Row | undefined;
      return row && now - row.at <= KEEP_MS ? row : undefined;
    } finally {
      db.close();
    }
  } catch {
    return undefined;
  }
}
