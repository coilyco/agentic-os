// A message the person sent, as the transcript shows it: the words apart from the image
// markers, and each image's upload path, shown from this device's copy when it holds one
// and from the host's thumbnail otherwise (COI-3800, references/seat-views.md).

/** `[Attached image: /path]` or `[Attached image 2 of 3: /path]`, from `imagePrefix`. */
const ATTACHED = /\[Attached image(?: \d+ of \d+)?: ([^\]\n]+)\][ \t]*/g;
/** A harness's marker for a pasted picture: `[Image #1]`, or opencode's `[Image 1]`. */
const MARKER = /\[Image #?\d+\][ \t]*/g;
/** A file the daemon named, typed bare by a seat with no bracketed paste. */
const BARE = /^(\/\S*\/images\/[^/\s]+\/[0-9a-f]+\.(?:png|jpg|gif|webp))(?:\s+|$)/;
/** Claude wraps a long paste in these, and the wrapper is not the person's words. */
const WRAPPER = /<\/?pasted_content(?: id="[^"]*")?>[ \t]*/g;

export interface SentMessage {
  words: string;
  paths: string[];
}

/** The words and the image paths of a sent message. */
export function parseSent(text: string): SentMessage {
  const paths: string[] = [];
  // A marker takes the spaces after it. Indentation inside the words is markdown's.
  let words = text.replace(ATTACHED, (_, path: string) => (paths.push(path.trim()), ""));
  words = words.replace(MARKER, "").replace(WRAPPER, "");
  // A bare path only leads, so a path the person wrote among their words stays words.
  for (let bare = BARE.exec(words.trimStart()); bare; bare = BARE.exec(words.trimStart())) {
    if (!paths.includes(bare[1]!)) paths.push(bare[1]!);
    words = words.trimStart().slice(bare[0].length);
  }
  return { words: words.trim(), paths };
}

/** The file's own name, which the viewer titles it with. */
export const fileName = (path: string): string => path.slice(path.lastIndexOf("/") + 1) || "image";

/** A refusal of an `image_thumb`, by the daemon's reason. */
export class ImageReadRefused extends Error {
  constructor(readonly reason: string) {
    super(readRefusal(reason));
  }
}

export function readRefusal(reason: string): string {
  if (reason === "image_gone") return "The host no longer holds this image. A seat's images go when it ends.";
  if (reason === "bad_path") return "This image is not in this seat's own uploads, so the host will not show it.";
  if (reason === "passkey_required") return "Unlock this device to see images.";
  return "The host would not send this image.";
}

/** The image type from the bytes, so a Blob from the host saves with its kind. */
export function sniffType(bytes: Uint8Array): string {
  const at = (index: number, ...values: number[]) => values.every((value, step) => bytes[index + step] === value);
  if (at(0, 0x89, 0x50, 0x4e, 0x47)) return "image/png";
  if (at(0, 0xff, 0xd8, 0xff)) return "image/jpeg";
  if (at(0, 0x47, 0x49, 0x46, 0x38)) return "image/gif";
  if (at(0, 0x52, 0x49, 0x46, 0x46) && at(8, 0x57, 0x45, 0x42, 0x50)) return "image/webp";
  return "application/octet-stream";
}

// This device's own copies, by upload path. A send fills it, so the picture shows with no
// round trip, pending or settled. Memory only and bounded, since a reload reads the host.
const LOCAL_HELD = 32;
const local = new Map<string, Blob>();

export function rememberSent(path: string, image: Blob): void {
  local.delete(path);
  local.set(path, image);
  while (local.size > LOCAL_HELD) local.delete(local.keys().next().value!);
}

export const localCopy = (path: string): Blob | undefined => local.get(path);

/** Clears what this device holds. Tests only. */
export function forgetSent(): void {
  local.clear();
  thumbs.clear();
}

// Thumbnails from the host, by session and path, so a view switch or a remount draws
// what it already read. A refusal that will not change is kept, anything else retried.
const THUMBS_HELD = 64;
const thumbs = new Map<string, Promise<Blob>>();

export function hostThumb(read: (session: string, path: string, full: boolean) => Promise<Blob>, session: string, path: string): Promise<Blob> {
  const key = `${session}|${path}`;
  const held = thumbs.get(key);
  if (held) return held;
  const asked = read(session, path, false);
  thumbs.set(key, asked);
  while (thumbs.size > THUMBS_HELD) thumbs.delete(thumbs.keys().next().value!);
  asked.catch((error: unknown) => {
    const settled = error instanceof ImageReadRefused && (error.reason === "image_gone" || error.reason === "bad_path");
    if (!settled && thumbs.get(key) === asked) thumbs.delete(key);
  });
  return asked;
}
