// A message just sent, shown in the transcript before the harness writes it (COI-3690).
import type { TranscriptItem } from "./transcript";

/** After this the entry says the seat did not take it. A late prompt still settles it. */
export const UNSENT_AFTER_MS = 30_000;
/** How far before the send a prompt's stamp may sit and still be it, for clock drift. */
export const SKEW_MS = 60_000;

export interface PendingSend {
  id: number;
  text: string;
  /** When the person pressed send, in ms on this device. */
  at: number;
  state: "pending" | "unsent";
}

const held = $state<Record<string, PendingSend[]>>({});
// A prompt resolves one send, so two sends of the same words stay two.
const claimed = new Set<string>();
const timers = new Map<number, ReturnType<typeof setTimeout>>();
let nextId = 1;

export const pendingFor = (key: string): readonly PendingSend[] => held[key] ?? [];

/** The words as one line, since a harness may fold a break. */
const flat = (text: string): string => text.replace(/\s+/g, " ").trim();

/** Equal, or the prompt ends with them: an image's path sits before the words. */
export function sameWords(sent: string, prompt: string): boolean {
  const a = flat(sent);
  const b = flat(prompt);
  return a !== "" && (b === a || b.endsWith(a));
}

export function notePending(key: string, text: string, at = Date.now()): number {
  const id = nextId++;
  held[key] = [...(held[key] ?? []), { id, text, at, state: "pending" }];
  timers.set(
    id,
    setTimeout(() => {
      timers.delete(id);
      const entry = held[key]?.find((one) => one.id === id);
      if (entry) entry.state = "unsent";
    }, UNSENT_AFTER_MS),
  );
  return id;
}

function drop(key: string, ids: ReadonlySet<number>): void {
  for (const id of ids) {
    clearTimeout(timers.get(id));
    timers.delete(id);
  }
  const left = (held[key] ?? []).filter((one) => !ids.has(one.id));
  if (left.length) held[key] = left;
  else delete held[key];
}

/** The send was refused and its words went back, or the person dismissed it. */
export function cancelPending(id: number): void {
  for (const key of Object.keys(held)) {
    if (held[key]?.some((one) => one.id === id)) drop(key, new Set([id]));
  }
}

/** Resolves what `items` carry. Fresh items are news whatever their stamp says.
 * A first read trusts only a stamp near or after the send. Returns the count. */
export function settlePending(key: string, items: readonly TranscriptItem[], fresh: boolean): number {
  const waiting = held[key];
  if (!waiting?.length) return 0;
  const done = new Set<number>();
  for (const send of waiting) {
    const hit = items.find(
      (item) =>
        item.kind === "prompt" &&
        item.text !== undefined &&
        !claimed.has(`${key}|${item.id}`) &&
        sameWords(send.text, item.text) &&
        (fresh || (item.at !== undefined && item.at >= send.at - SKEW_MS)),
    );
    if (!hit) continue;
    claimed.add(`${key}|${hit.id}`);
    done.add(send.id);
  }
  if (done.size) drop(key, done);
  return done.size;
}
