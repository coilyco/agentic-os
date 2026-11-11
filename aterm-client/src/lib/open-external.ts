// Opens an address in the device's own browser (COI-3520). A Tauri window ignores
// `window.open`, so the app asks its opener plugin. A browser opens a tab.
import { bridge, type Invoke } from "./device-key";
import { pageLabel } from "./pages";

const COMMAND = "plugin:opener|open_url";

export type Opened = { ok: true } | { ok: false; message: string };

export interface OpenIo {
  invoke: Invoke | null;
  tab(url: string): void;
}

const defaults = (): OpenIo => ({ invoke: bridge(), tab: (url) => void window.open(url, "_blank", "noopener,noreferrer") });

/** Inside the Android or Mac app, where the page has the app's bridge. */
export function inApp(io: Pick<OpenIo, "invoke"> = defaults()): boolean {
  return io.invoke !== null;
}

/** The Android app on a phone, where no host's own browser opens (COI-3559). */
export function inAndroidApp(io: Pick<OpenIo, "invoke"> = defaults(), agent: string = globalThis.navigator?.userAgent ?? ""): boolean {
  return inApp(io) && /Android/i.test(agent);
}

/** A claude.ai artifact, which the seat's streamed browser cannot show. */
export function isArtifactUrl(url: string): boolean {
  try {
    const { protocol, hostname, pathname } = new URL(url);
    return protocol === "https:" && (hostname === "claude.ai" || hostname === "www.claude.ai") && /^\/(code\/)?artifact\/[^/]+/.test(pathname);
  } catch {
    return false;
  }
}

function detail(error: unknown): string {
  const text = typeof error === "string" ? error : ((error as { message?: unknown } | null)?.message ?? "");
  return typeof text === "string" ? text.trim().slice(0, 120) : "";
}

/** Opens a web address in the device's browser, and says in words when it could not. */
export async function openExternal(url: string, io: OpenIo = defaults()): Promise<Opened> {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return { ok: false, message: "That is not a web address, so it was not opened." };
  }
  if (parsed.protocol !== "https:" && parsed.protocol !== "http:") return { ok: false, message: "Only web addresses open from here, so that one was not opened." };
  if (!io.invoke) {
    io.tab(url);
    return { ok: true };
  }
  try {
    await io.invoke(COMMAND, { url });
    return { ok: true };
  } catch (error) {
    const why = detail(error);
    return { ok: false, message: `${pageLabel(url)} did not open in the browser${why ? `: ${why}` : ""}. The address is on the card to copy.` };
  }
}
