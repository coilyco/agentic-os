// Files a seat shares with the client (COI-3458): images, video, and anything else.
// The daemon owns the frames. The client shows them and never decides what a file is.
import { formatSize } from "./images";

export type MediaKind = "image" | "video" | "file";

export interface MediaItem {
  id: string;
  session: string;
  name: string;
  mime: string;
  kind: MediaKind;
  size: number;
  /** Epoch milliseconds, when the seat shared it. */
  at?: number;
}

/** The host keeps this many a seat, and so does the shelf. */
export const MAX_SHELF = 32;
/** One `media_read` asks for at most this many bytes, as a frame is capped near 4 MiB. */
export const CHUNK_BYTES = 2_000_000;

const KINDS: readonly string[] = ["image", "video", "file"];

export function decodeMedia(session: string, raw: unknown): MediaItem | null {
  if (typeof raw !== "object" || raw === null) return null;
  const item = raw as Record<string, unknown>;
  const id = typeof item.id === "string" && item.id !== "" ? item.id : undefined;
  if (!id || typeof item.name !== "string" || !KINDS.includes(String(item.kind))) return null;
  const size = typeof item.size === "number" && Number.isFinite(item.size) && item.size >= 0 ? item.size : 0;
  const at = typeof item.at === "string" ? Date.parse(item.at) : typeof item.at === "number" ? item.at : NaN;
  return {
    id,
    session,
    name: item.name || "Unnamed file",
    mime: typeof item.mime === "string" && item.mime ? item.mime : "application/octet-stream",
    kind: item.kind as MediaKind,
    size,
    ...(Number.isFinite(at) ? { at } : {}),
  };
}

/** A new item goes last, a repeat of an id replaces in place, the newest 32 stay. */
export function addToShelf(shelf: readonly MediaItem[], item: MediaItem): MediaItem[] {
  const at = shelf.findIndex((each) => each.id === item.id);
  const next = at === -1 ? [...shelf, item] : shelf.map((each, index) => (index === at ? item : each));
  return next.slice(-MAX_SHELF);
}

export const removeFromShelf = (shelf: readonly MediaItem[], id: string): MediaItem[] => shelf.filter((item) => item.id !== id);

/** How many items came after the last one looked at. All of them, when none was. */
export function unseenCount(shelf: readonly MediaItem[], seenId: string | undefined): number {
  const at = seenId === undefined ? -1 : shelf.findIndex((item) => item.id === seenId);
  return shelf.length - 1 - at;
}

/** One step through a sequence, stopping at the ends. */
export function stepped(index: number, length: number, direction: 1 | -1): number {
  return Math.min(length - 1, Math.max(0, index + direction));
}

/** A quarter turn, so a camera frame that came out upside down takes two. */
export function turned(degrees: number, direction: 1 | -1 = 1): number {
  return (((degrees + 90 * direction) % 360) + 360) % 360;
}

const REFUSALS: Record<string, string> = {
  media_unknown: "The host no longer has that file. It keeps the newest 32 a seat shares.",
  too_large: "That file is larger than the host will send.",
  locked: "This device may not open a seat's files. It can read the screen.",
};

export function mediaRefusal(reason: unknown): string {
  return (typeof reason === "string" && Object.hasOwn(REFUSALS, reason) && REFUSALS[reason]) || "The host would not send that file.";
}

/** The line under a name: what it is and how big. */
export function describe(item: MediaItem): string {
  const kind = item.kind === "image" ? "Image" : item.kind === "video" ? "Video" : "File";
  return item.size > 0 ? `${kind}, ${formatSize(item.size)}` : kind;
}
