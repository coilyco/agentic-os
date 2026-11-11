// Saves a file the page holds into the device's Downloads, through the app's savefile
// plugin (COI-3593). A WebView drops a blob download, so the bytes go over in pieces.
// Shapes: models.rs in the savefile plugin. Design: references/save-file.md.
import { bridge, type Invoke } from "./device-key";
import { formatSize } from "./images";

const PLUGIN = "plugin:savefile|";

/** One piece's size. Base64 adds a third on the bridge, so this stays small. */
export const SAVE_CHUNK_BYTES = 384 * 1024;

export type SaveResult = { ok: true; message: string } | { ok: false; message: string };

const str = (value: unknown): string => (typeof value === "string" ? value : "");

/** Where a save lands in words, or null in a browser and in an app without the plugin. */
export async function saveAvailable(invoke: Invoke | null = bridge()): Promise<string | null> {
  if (!invoke) return null;
  try {
    const place = str(((await invoke(`${PLUGIN}available`)) as { place?: unknown } | null)?.place);
    return place || null;
  } catch {
    return null;
  }
}

function base64(bytes: Uint8Array): string {
  let text = "";
  for (let at = 0; at < bytes.length; at += 0x8000) text += String.fromCharCode(...bytes.subarray(at, at + 0x8000));
  return btoa(text);
}

function why(error: unknown): string {
  const { code, message } = (error ?? {}) as { code?: unknown; message?: unknown };
  switch (code) {
    case "busy":
      return "another save is still open";
    case "no_folder":
      return "this device has no Downloads folder";
    default: {
      const text = str(message) || (typeof error === "string" ? error : "");
      return text.trim().slice(0, 120);
    }
  }
}

/** Writes `blob` as `name`, calling `progress` per piece. A failure leaves no file. */
export async function saveInApp(blob: Blob, name: string, invoke: Invoke, progress?: (done: number, total: number) => void): Promise<SaveResult> {
  let id = "";
  try {
    const begun = (await invoke(`${PLUGIN}begin`, { name, mime: blob.type })) as { id?: unknown } | null;
    id = str(begun?.id);
    if (!id) throw new Error("The app did not open a file to save into.");
    for (let at = 0; at < blob.size; at += SAVE_CHUNK_BYTES) {
      const piece = new Uint8Array(await blob.slice(at, at + SAVE_CHUNK_BYTES).arrayBuffer());
      await invoke(`${PLUGIN}write`, { id, data: base64(piece) });
      progress?.(Math.min(at + SAVE_CHUNK_BYTES, blob.size), blob.size);
    }
    const saved = (await invoke(`${PLUGIN}finish`, { id })) as { name?: unknown; place?: unknown } | null;
    return { ok: true, message: `Saved ${str(saved?.name) || name} to ${str(saved?.place) || "Downloads"}.` };
  } catch (error) {
    if (id) await Promise.resolve(invoke(`${PLUGIN}cancel`, { id })).catch(() => undefined);
    const reason = why(error);
    return { ok: false, message: `${name} (${formatSize(blob.size)}) was not saved${reason ? `: ${reason}` : ""}. Nothing was left in Downloads.` };
  }
}
