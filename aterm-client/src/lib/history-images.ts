// The images of past sends, so a send can be resent as it was (COI-3705). Words live in
// send-history.ts. A File cannot sit in local storage, so the bytes sit in IndexedDB.
import { KEEP_MS } from "./send-history";

const DB_NAME = "aterm.send-images.v1";
const STORE = "sends";

/** Image sends kept per seat. Ten is prod-director's starting cap (COI-3705). */
export const PER_SEAT_SENDS = 10;
/** All a device keeps across seats. Ten sends of four 3 MB images is 120 MB for one. */
export const MAX_BYTES = 48 * 1024 * 1024;

export interface Row {
  /** `seat key|send id`. */
  key: string;
  seat: string;
  at: number;
  bytes: number;
  // Bytes, not a Blob: WebKit will not store a Blob in a private session.
  files: { name: string; type: string; lastModified: number; data: ArrayBuffer }[];
}

export const rowKey = (seat: string, id: string): string => `${seat}|${id}`;

/** Rows to drop: stale ones, a seat's past the cap, then the oldest until it fits. */
export function planKeep(rows: Pick<Row, "key" | "seat" | "at" | "bytes">[], now: number): string[] {
  const drop = new Set(rows.filter((row) => now - row.at > KEEP_MS).map((row) => row.key));
  const bySeat = new Map<string, typeof rows>();
  for (const row of rows) if (!drop.has(row.key)) bySeat.set(row.seat, [...(bySeat.get(row.seat) ?? []), row]);
  for (const list of bySeat.values()) {
    for (const row of [...list].sort((a, b) => b.at - a.at).slice(PER_SEAT_SENDS)) drop.add(row.key);
  }
  const live = rows.filter((row) => !drop.has(row.key)).sort((a, b) => a.at - b.at);
  let total = live.reduce((sum, row) => sum + row.bytes, 0);
  for (const row of live) {
    if (total <= MAX_BYTES) break;
    drop.add(row.key);
    total -= row.bytes;
  }
  return [...drop];
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

/** Keep these files for one send. False when the device would not take them. */
export async function saveSendImages(seat: string, id: string, files: File[], now = Date.now()): Promise<boolean> {
  try {
    const bytes = files.reduce((sum, file) => sum + file.size, 0);
    if (bytes > MAX_BYTES) return false;
    // Read first: a transaction ends while it waits on anything but its own requests.
    const data = await Promise.all(files.map((file) => file.arrayBuffer()));
    await inStore("readwrite", async (store) => {
      const row: Row = { key: rowKey(seat, id), seat, at: now, bytes, files: files.map((file, i) => ({ name: file.name, type: file.type, lastModified: file.lastModified, data: data[i] as ArrayBuffer })) };
      await wrap(store.put(row));
      const rows = (await wrap(store.getAll())) as Row[];
      for (const old of planKeep(rows, now)) if (old !== row.key) await wrap(store.delete(old));
    });
    return true;
  } catch {
    return false;
  }
}

export async function loadSendImages(seat: string, id: string): Promise<File[]> {
  try {
    const row = await inStore("readonly", (store) => wrap(store.get(rowKey(seat, id)) as IDBRequest<Row | undefined>));
    return (row?.files ?? []).map((each) => new File([each.data], each.name, { type: each.type, lastModified: each.lastModified }));
  } catch {
    return [];
  }
}

/** Drop a seat's rows that no entry in its history points at any more. */
export async function keepOnly(seat: string, ids: readonly string[]): Promise<void> {
  try {
    const wanted = new Set(ids.map((id) => rowKey(seat, id)));
    await inStore("readwrite", async (store) => {
      const rows = (await wrap(store.getAll())) as Row[];
      for (const row of rows) if (row.seat === seat && !wanted.has(row.key)) await wrap(store.delete(row.key));
    });
  } catch {
    // Storage is off. There is nothing kept to drop.
  }
}
