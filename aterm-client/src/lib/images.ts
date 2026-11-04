// Images in the composer: what is accepted, the words when one is not, and how a
// path reaches the seat. The host half is the daemon's (COI-3239), so nothing here
// names a frame. `ImageChannel` in protocol.ts is the whole seam.
import type { ImageChannel } from "./protocol";

export const IMAGE_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"] as const;

/** Most images one message carries. A phone link uploads them one after another. */
export const MAX_IMAGES = 4;

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** What a screen reader hears for a chip, and what is read beside its picture. */
export function chipLabel(index: number, bytes: number): string {
  return `Image ${index + 1}, ${formatSize(bytes)}`;
}

/** Images from a paste, or none when text came with them, so a cell stays text. */
export function pastedImages(data: Pick<DataTransfer, "files" | "types"> | null): File[] {
  if (!data) return [];
  if (Array.from(data.types).includes("text/plain")) return [];
  return Array.from(data.files).filter((file) => file.type.startsWith("image/"));
}

export interface Screened {
  accepted: File[];
  /** Why something was left out, in words a person can act on. Null when nothing was. */
  refusal: string | null;
}

/** Sorts files into what the composer takes and the first reason it refused any. */
export function screenImages(files: readonly File[], waiting: number, channel: Pick<ImageChannel, "maxBytes"> | undefined): Screened {
  if (files.length === 0) return { accepted: [], refusal: null };
  if (!channel) return { accepted: [], refusal: "This host cannot take images, so nothing was added. It needs a newer aterm daemon." };
  const accepted: File[] = [];
  let refusal: string | null = null;
  for (const file of files) {
    if (!(IMAGE_TYPES as readonly string[]).includes(file.type)) {
      refusal ??= `${file.name || "That file"} is not an image the seat can read. PNG, JPEG, GIF and WebP work.`;
    } else if (file.size > channel.maxBytes) {
      refusal ??= `${file.name || "That image"} is ${formatSize(file.size)}, over the ${formatSize(channel.maxBytes)} this host takes.`;
    } else if (waiting + accepted.length >= MAX_IMAGES) {
      refusal ??= `A message carries ${MAX_IMAGES} images at most. Send these, then add more.`;
    } else {
      accepted.push(file);
    }
  }
  return { accepted, refusal };
}

/** A path as a terminal types a dropped file: backslash before shell syntax. */
// Whether each harness reads that as an attachment is the live check in COI-3239.
export function typedPath(path: string): string {
  return path.replace(/[ \t'"()[\]{}&;$!#<>|*?\\]/g, "\\$&");
}

/** The text a send types: each image's path, then what the person wrote. */
export function withImagePaths(body: string, paths: readonly string[]): string {
  const lead = paths.map(typedPath).join(" ");
  if (!lead) return body;
  return body ? `${lead} ${body}` : lead;
}
