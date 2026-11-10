// Images attached to an unsent message outlive a closed app (COI-3582), beside the
// draft's words in draft-store.ts. A File cannot sit in local storage: IndexedDB.
import { KEEP_MS } from "./draft-store";

const DB_NAME = "aterm.draft-images.v1";
const STORE = "images";

/** All a device keeps across seats. The host's per-image limit still applies to each. */
export const MAX_BYTES = 12 * 1024 * 1024;

export type Kept = "kept" | "too_big" | "off";

export interface Row {
  key: string;
  at: number;
  bytes: number;
  files: { name: string; type: string; lastModified: number; blob: Blob }[];
}

/** The keys to drop: the stale ones, then the oldest until the rest fit the budget. */
export function planPrune(rows: Pick<Row, "key" | "at" | "bytes">[], keep: string, now: number): string[] {
  const drop = rows.filter((row) => row.key !== keep && now - row.at > KEEP_MS).map((row) => row.key);
  const live = rows.filter((row) => !drop.includes(row.key)).sort((a, b) => a.at - b.at);
  let total = live.reduce((sum, row) => sum + row.bytes, 0);
  for (const row of live) {
    if (total <= MAX_BYTES) break;
    if (row.key === keep) continue;
    drop.push(row.key);
    total -= row.bytes;
  }
  return drop;
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

/** Keep these images for a seat's draft, or none. An empty list clears them. */
export async function saveDraftImages(key: string, files: File[], now = Date.now()): Promise<Kept> {
  try {
    const bytes = files.reduce((sum, file) => sum + file.size, 0);
    const fits = bytes <= MAX_BYTES;
    await inStore("readwrite", async (store) => {
      if (files.length === 0 || !fits) {
        await wrap(store.delete(key));
        return;
      }
      const row: Row = { key, at: now, bytes, files: files.map((file) => ({ name: file.name, type: file.type, lastModified: file.lastModified, blob: file })) };
      await wrap(store.put(row));
      const rows = (await wrap(store.getAll())) as Row[];
      for (const old of planPrune(rows, key, now)) await wrap(store.delete(old));
    });
    return fits ? "kept" : "too_big";
  } catch {
    return files.length === 0 ? "kept" : "off";
  }
}

export async function loadDraftImages(key: string, now = Date.now()): Promise<File[]> {
  try {
    const row = await inStore("readonly", (store) => wrap(store.get(key) as IDBRequest<Row | undefined>));
    if (!row || now - row.at > KEEP_MS) return [];
    return row.files.map((each) => new File([each.blob], each.name, { type: each.type, lastModified: each.lastModified }));
  } catch {
    return [];
  }
}
