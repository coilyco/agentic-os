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

const ASKED_KEY = "aterm.notifyAsked.v1";

/** True once this device has shown the system prompt. Android reports a first no as
 * "never asked", so the system alone lets the prompt return on every launch. */
export function askedBefore(): boolean {
  try {
    return localStorage.getItem(ASKED_KEY) === "1";
  } catch {
    return false;
  }
}

export function rememberAsked(): void {
  try {
    localStorage.setItem(ASKED_KEY, "1");
  } catch {
    // Storage can be off. The prompt then returns at the next launch, as it used to.
  }
}

/** The prompt is a system dialog that covers the page and eats taps, so it waits for a
 * screen with no composer: not a seat, not a launch form. The demo never asks. */
export function mayAskNow(on: { attached: boolean; visible: boolean; composerShown: boolean; demo: boolean }): boolean {
  return on.attached && on.visible && !on.composerShown && !on.demo;
}

/** One notification per seat: a new one from the same seat replaces the old. */
export function noticeId(session: string): number {
  let hash = 2166136261;
  for (const char of session) hash = Math.imul(hash ^ char.charCodeAt(0), 16777619);
  return (hash >>> 0) % 2147483646 + 1;
}

/** True in the Android app's WebView. Only Android names a small icon. */
export const onAndroid = (): boolean => typeof navigator !== "undefined" && /Android/i.test(navigator.userAgent);

/** The status bar's flat canopy, a drawable the icons script writes (COI-3613). */
export const SMALL_ICON = "ic_stat_treehouse";

export function nativeNotice(note: Notify, android = onAndroid()): { options: { id: number; title: string; body: string; extra: { session: string }; icon?: string } } {
  const options = { id: noticeId(note.session), title: note.title, body: note.body, extra: { session: note.session } };
  return { options: android ? { ...options, icon: SMALL_ICON } : options };
}

/** What was raised and not yet tapped, by notification id. A tap carries no notification
 * on Android (the plugin never fills it), so this is how a tap finds its seat. */
export type Noted = Record<string, { session: string; at: number }>;

const NOTED_KEY = "aterm.notified.v1";
const NOTED_KEEP_MS = 24 * 60 * 60 * 1000;

export function parseNoted(text: string | null, now: number): Noted {
  try {
    const raw: unknown = JSON.parse(text ?? "null");
    if (typeof raw !== "object" || raw === null) return {};
    const kept: Noted = {};
    for (const [id, entry] of Object.entries(raw)) {
      const { session, at } = (entry ?? {}) as { session?: unknown; at?: unknown };
      if (typeof session === "string" && typeof at === "number" && now - at < NOTED_KEEP_MS) kept[id] = { session, at };
    }
    return kept;
  } catch {
    return {};
  }
}

function loadNoted(now: number): Noted {
  try {
    return parseNoted(localStorage.getItem(NOTED_KEY), now);
  } catch {
    return {};
  }
}

function storeNoted(noted: Noted): void {
  try {
    localStorage.setItem(NOTED_KEY, JSON.stringify(noted));
  } catch {
    // Storage can be off. A tap then opens nothing, and the app is in front.
  }
}

/** The seat of the notification a tap dismissed: the newest one raised that is no longer
 * showing, since the plugin dismisses the tapped one first. Null when none is missing. */
export function tappedFromActive(noted: Noted, active: ReadonlySet<number>): { id: string; session: string } | null {
  const gone = Object.entries(noted)
    .filter(([id]) => !active.has(Number(id)))
    .sort((a, b) => b[1].at - a[1].at)[0];
  return gone ? { id: gone[0], session: gone[1].session } : null;
}

async function activeIds(core: Core): Promise<Set<number> | null> {
  try {
    const list = await core.invoke("plugin:notification|get_active");
    return Array.isArray(list) ? new Set(list.map((each) => Number((each as { id?: unknown }).id))) : null;
  } catch {
    return null;
  }
}

export async function showNative(core: Core, note: Notify, now = Date.now()): Promise<boolean> {
  try {
    await core.invoke("plugin:notification|notify", nativeNotice(note));
  } catch {
    return false;
  }
  const noted = loadNoted(now);
  noted[String(noticeId(note.session))] = { session: note.session, at: now };
  storeNoted(noted);
  return true;
}

/** The seat a tapped notification names. The plugin sends the tap as `actionPerformed`
 * with the notification and its extras, on Android only. Anything else names no seat. */
export function tappedSeat(payload: unknown): string | null {
  const extra = (payload as { notification?: { extra?: { session?: unknown } } } | null)?.notification?.extra;
  return typeof extra?.session === "string" && extra.session !== "" ? extra.session : null;
}

/** The seat of a tap: the payload's, if the plugin fills it, else from the record. */
export async function seatOfTap(core: Core, payload: unknown, now = Date.now()): Promise<string | null> {
  const named = tappedSeat(payload);
  if (named) return named;
  const noted = loadNoted(now);
  const active = await activeIds(core);
  const found = active ? tappedFromActive(noted, active) : null;
  if (!found) return null;
  delete noted[found.id];
  storeNoted(noted);
  return found.session;
}

/** Calls `open` with the seat of each tapped notification. Returns how to stop. */
export async function listenForTaps(core: Core, open: (session: string) => void): Promise<() => void> {
  if (!core.addPluginListener) return () => {};
  try {
    const listener = await core.addPluginListener("notification", "actionPerformed", (payload) => {
      void seatOfTap(core, payload).then((session) => session && open(session));
    });
    return () => void listener.unregister().catch(() => {});
  } catch {
    return () => {};
  }
}
