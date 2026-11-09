// Images in the composer: what is accepted, the words when one is not, and how a
// path reaches the seat. `ImageChannel` in protocol.ts is the seam, and the daemon's
// side is references/image-upload.md (COI-3239).
import type { ImageChannel } from "./protocol";

export const IMAGE_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"] as const;

/** Most images one message carries. A phone link uploads them one after another. */
export const MAX_IMAGES = 4;

/** Set by the frame cap: base64 is 4/3 as long, and over 4 MiB drops the connection. */
export const IMAGE_UPLOAD_MAX_BYTES = 3_000_000;

/** Harnesses that read a pasted path as text only, observed 2026-11-04. */
const NO_ATTACH = ["goose"];

export function attachNote(harness: string): string | null {
  return NO_ATTACH.includes(harness) ? `${harness} does not attach images, so it got the file's path to open instead.` : null;
}

/** The daemon's refusal of an `image_upload` in words, or null for another reason. */
export function imageRefusal(reason: unknown): string | null {
  switch (reason) {
    case "not_an_image":
      return "The host did not read that as a PNG, JPEG, GIF or WebP image.";
    case "too_large":
      return `The host took that as over ${formatSize(IMAGE_UPLOAD_MAX_BYTES)}.`;
    case "too_many":
      return "This seat already holds 16 images, the most the host keeps for one. Ending the seat clears them.";
    case "session_unknown":
      return "The host is not attached to this seat, so it could not take the image. Try again in a moment.";
    default:
      return null;
  }
}

export function formatSize(bytes: number): string {
  // Decimal, as Finder says it, so the host's 3,000,000 byte limit reads as 3 MB.
  if (bytes < 1000) return `${bytes} B`;
  if (bytes < 1_000_000) return `${Math.max(1, Math.round(bytes / 1000))} KB`;
  return `${(bytes / 1_000_000).toFixed(1).replace(/\.0$/, "")} MB`;
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
// A still image over the limit is taken here and shrunk by `fitImage`. A GIF is refused.
export function screenImages(files: readonly File[], waiting: number, channel: Pick<ImageChannel, "maxBytes"> | undefined): Screened {
  if (files.length === 0) return { accepted: [], refusal: null };
  if (!channel) return { accepted: [], refusal: "This host cannot take images, so nothing was added. It needs a newer aterm daemon." };
  const accepted: File[] = [];
  let refusal: string | null = null;
  for (const file of files) {
    if (!(IMAGE_TYPES as readonly string[]).includes(file.type)) {
      refusal ??= `${file.name || "That file"} is not an image the seat can read. PNG, JPEG, GIF and WebP work.`;
    } else if (file.size > channel.maxBytes && file.type === "image/gif") {
      refusal ??= `${file.name || "That GIF"} is ${formatSize(file.size)}, over the ${formatSize(channel.maxBytes)} this host takes, and a GIF cannot be shrunk without losing its motion.`;
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

/** Names each image and its path, so a seat finds the file if the attachment is late. */
export function imagePrefix(paths: readonly string[]): string {
  return paths.map((path, index) => `[Attached image${paths.length > 1 ? ` ${index + 1} of ${paths.length}` : ""}: ${path}]`).join(" ");
}

/** Bracketed pastes for a send. A path goes alone, since that paste is what attaches. */
export function pasteSteps(paths: readonly string[], text: string, open: string, close: string): string[] {
  const steps = paths.map((path) => `${open}${path}${close}`);
  const words = [imagePrefix(paths), text].filter(Boolean).join(" ");
  if (words) steps.push(`${open}${words}${close}`);
  return steps;
}

/** The encodings `fitImage` tries in order. A smaller PNG keeps screenshot text sharp. */
export function fitAttempts(type: string): { scale: number; type: "image/png" | "image/jpeg" }[] {
  const jpeg = [1, 0.75, 0.5].map((scale) => ({ scale, type: "image/jpeg" as const }));
  if (type !== "image/png") return jpeg;
  return [...[0.75, 0.5].map((scale) => ({ scale, type: "image/png" as const })), ...jpeg];
}

function drawn(bitmap: ImageBitmap, scale: number, type: string): Promise<Blob | null> {
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(bitmap.width * scale));
  canvas.height = Math.max(1, Math.round(bitmap.height * scale));
  const context = canvas.getContext("2d");
  if (!context) return Promise.resolve(null);
  // A JPEG has no transparency, and an unfilled one comes out black.
  if (type === "image/jpeg") {
    context.fillStyle = "#fff";
    context.fillRect(0, 0, canvas.width, canvas.height);
  }
  context.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
  return new Promise((resolve) => canvas.toBlob(resolve, type, 0.88));
}

/** A copy of a still image under `maxBytes`, or null when no encoding fits. */
export async function fitImage(file: File, maxBytes: number): Promise<File | null> {
  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file);
  } catch {
    return null;
  }
  try {
    for (const attempt of fitAttempts(file.type)) {
      const blob = await drawn(bitmap, attempt.scale, attempt.type);
      if (blob && blob.size <= maxBytes) {
        const extension = attempt.type === "image/png" ? "png" : "jpg";
        return new File([blob], `${file.name.replace(/\.[^./]+$/, "") || "image"}.${extension}`, { type: attempt.type });
      }
    }
    return null;
  } finally {
    bitmap.close();
  }
}
