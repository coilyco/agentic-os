// The Mac and Android apps show a coordinator's notify through Tauri's notification
// plugin (COI-3578). A browser has no such bridge, so it keeps the Notification API.
import type { Notify } from "./protocol";

interface Core {
  invoke: (command: string, args?: unknown) => Promise<unknown>;
  addPluginListener?: (plugin: string, event: string, callback: (payload: unknown) => void) => Promise<{ unregister: () => Promise<void> }>;
}

/** The plugin bridge when this page is the app, else null. */
export function nativeCore(scope: object = globalThis): Core | null {
  const core = (scope as { __TAURI__?: { core?: Core } }).__TAURI__?.core;
  return typeof core?.invoke === "function" ? core : null;
}

export type Permission = "granted" | "denied" | "ask";

/** What the system says now. Null means never asked. A failed call reads as denied. */
export async function permissionOf(core: Core): Promise<Permission> {
  try {
    const granted = await core.invoke("plugin:notification|is_permission_granted");
    return granted === true ? "granted" : granted === false ? "denied" : "ask";
  } catch {
    return "denied";
  }
}

/** Shows the system's own prompt. True only when the person allowed it. */
export async function askPermission(core: Core): Promise<boolean> {
  try {
    return (await core.invoke("plugin:notification|request_permission")) === "granted";
  } catch {
    return false;
  }
}

/** The seat's own words, with the seat named in the extras so a tap can open it. */
export function nativeNotice(note: Notify): { options: { title: string; body: string; extra: { session: string } } } {
  return { options: { title: note.title, body: note.body, extra: { session: note.session } } };
}

export async function showNative(core: Core, note: Notify): Promise<boolean> {
  try {
    await core.invoke("plugin:notification|notify", nativeNotice(note));
    return true;
  } catch {
    return false;
  }
}

/** The seat a tapped notification names. The plugin sends the tap as `actionPerformed`
 * with the notification and its extras, on Android only. Anything else names no seat. */
export function tappedSeat(payload: unknown): string | null {
  const extra = (payload as { notification?: { extra?: { session?: unknown } } } | null)?.notification?.extra;
  return typeof extra?.session === "string" && extra.session !== "" ? extra.session : null;
}

/** Calls `open` with the seat of each tapped notification. Returns how to stop. */
export async function listenForTaps(core: Core, open: (session: string) => void): Promise<() => void> {
  if (!core.addPluginListener) return () => {};
  try {
    const listener = await core.addPluginListener("notification", "actionPerformed", (payload) => {
      const session = tappedSeat(payload);
      if (session) open(session);
    });
    return () => void listener.unregister().catch(() => {});
  } catch {
    return () => {};
  }
}
