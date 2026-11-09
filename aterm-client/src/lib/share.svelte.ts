// The shared images waiting for a seat, and when to ask the plugin for more.
import { MAX_IMAGES } from "./images";
import { skipWords, takeShared, type Shared } from "./share-target";

export const share = $state({
  /** Shared images and words no seat has taken yet. */
  pending: null as Shared | null,
  open: false,
});

/** Adds a take to what waits, so a second share while the sheet is open loses nothing. */
export function merge(pending: Shared | null, next: Shared): Shared {
  if (!pending) return { files: next.files.slice(0, MAX_IMAGES), text: next.text, notes: [...next.notes, ...next.files.slice(MAX_IMAGES).map((file) => skipWords(file.name, "too_many"))] };
  const room = Math.max(0, MAX_IMAGES - pending.files.length);
  return {
    files: [...pending.files, ...next.files.slice(0, room)],
    text: [pending.text, next.text].filter(Boolean).join("\n"),
    notes: [...pending.notes, ...next.notes, ...next.files.slice(room).map((file) => skipWords(file.name, "too_many"))],
  };
}

/** Asks the plugin what Android handed over, and opens the sheet if there is any. */
export async function checkShares(take: typeof takeShared = takeShared): Promise<void> {
  const got = await take();
  if (!got) return;
  share.pending = merge(share.pending, got);
  share.open = true;
}

export function discard(): void {
  share.pending = null;
  share.open = false;
}
