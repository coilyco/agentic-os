// Images Android's Share menu handed the app, read from the plugin's `take`.
// Shapes: models.rs in the sharetarget plugin. Design: references/android-sharetarget.md.
import { bridge, type Invoke } from "./device-key";
import { IMAGE_TYPES, MAX_IMAGES, formatSize } from "./images";

const COMMAND = "plugin:sharetarget|take";

/** The plugin's read cap per item, `ShareRules.MAX_BYTES` in Kotlin. */
export const SHARE_READ_MAX_BYTES = 25_000_000;

export type SkipReason = "not_image" | "too_big" | "unreadable" | "too_many";
const REASONS: readonly SkipReason[] = ["not_image", "too_big", "unreadable", "too_many"];

/** What was shared: files, the words that came with them, and what was left out. */
export interface Shared {
  files: File[];
  text: string;
  notes: string[];
}

export function skipWords(name: string, reason: SkipReason): string {
  switch (reason) {
    case "not_image":
      return `${name} is not an image the seat can read, so it was left out. PNG, JPEG, GIF and WebP work.`;
    case "too_big":
      return `${name} is over ${formatSize(SHARE_READ_MAX_BYTES)}, too big to take from the Share menu.`;
    case "unreadable":
      return `${name} could not be read. The app that shared it may have taken it back, so share it again.`;
    case "too_many":
      return `A message carries ${MAX_IMAGES} images at most, so ${name} was left out.`;
  }
}

function decode(data: string, name: string, mime: string): File {
  const raw = atob(data);
  const bytes = new Uint8Array(raw.length);
  for (let index = 0; index < raw.length; index++) bytes[index] = raw.charCodeAt(index);
  return new File([bytes], name, { type: mime });
}

const str = (value: unknown): string => (typeof value === "string" ? value : "");

/** The answer as files and words, or null if none waited. A bad item is a note. */
export function parseShared(raw: unknown): Shared | null {
  const { images, skipped, text } = (raw ?? {}) as { images?: unknown; skipped?: unknown; text?: unknown };
  const files: File[] = [];
  const notes: string[] = [];
  for (const item of Array.isArray(images) ? images : []) {
    const { name, mime, data } = (item ?? {}) as Record<string, unknown>;
    const label = str(name) || "An image";
    try {
      if (!str(data) || !str(mime)) throw new Error("empty");
      // The plugin passes any image type. `IMAGE_TYPES` is the one list a seat reads.
      if (!(IMAGE_TYPES as readonly string[]).includes(str(mime))) notes.push(skipWords(label, "not_image"));
      else files.push(decode(str(data), label, str(mime)));
    } catch {
      notes.push(skipWords(label, "unreadable"));
    }
  }
  for (const item of Array.isArray(skipped) ? skipped : []) {
    const { name, reason } = (item ?? {}) as Record<string, unknown>;
    const known = REASONS.find((each) => each === reason) ?? "unreadable";
    notes.push(skipWords(str(name) || "An item", known));
  }
  const words = str(text).trim();
  if (files.length === 0 && notes.length === 0 && !words) return null;
  return { files, text: words, notes };
}

// Null in a browser, an app without the plugin, and on failure. A failure looks like an
// old app, and a sheet on every launch is worse than a repeat share.
export async function takeShared(invoke: Invoke | null = bridge()): Promise<Shared | null> {
  if (!invoke) return null;
  try {
    return parseShared(await invoke(COMMAND));
  } catch (error) {
    console.warn("share target: take failed", error);
    return null;
  }
}
