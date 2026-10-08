// Opt-in Web Push for a seat that starts waiting while the app is closed (COI-2559).
// The frames and payload: .agents/skills/tooling-aterm-client/references/web-push.md
import type { PushChannel } from "./protocol";
import { fromBase64Url } from "./passkey";
import { Refusal } from "./typing";

/** A failure already in words for the person. */
export class PushError extends Error {}

const KEY = "aterm.push.v1";

/** The notification tag the daemon sends: one per seat. */
export const seatTag = (session: string) => `aterm-${session}`;

/** How long a tapped notification waits for its host to attach and list the seat. */
export const SEAT_WAIT_MS = 15000;

export const DENIED = "Notifications are blocked for this site. Allow them in the browser's site settings, then switch this on again.";
const DISMISSED = "Notifications were not allowed, so nothing was turned on. Switch this on again to be asked.";
const NO_WORKER = "The page is not ready for notifications yet. Reload it, then switch this on again.";

/** What the browser exposes that push needs, read once so a test can hand in its own. */
export interface PushGlobals {
  secure: boolean;
  production: boolean;
  worker: boolean;
  manager: boolean;
  notification: boolean;
}

export function pushGlobals(): PushGlobals {
  return {
    secure: typeof isSecureContext === "undefined" || isSecureContext,
    production: import.meta.env.PROD,
    worker: typeof navigator !== "undefined" && "serviceWorker" in navigator,
    manager: typeof window !== "undefined" && "PushManager" in window,
    notification: typeof Notification !== "undefined",
  };
}

/** Why this page cannot receive push at all, or null. Checked before asking. */
export function pushUnavailable(globals: PushGlobals = pushGlobals()): string | null {
  if (!globals.secure) return "Notifications need a secure page, and this one is not.";
  if (!globals.worker || !globals.manager || !globals.notification) {
    return "This browser cannot show notifications while the app is closed. On iPhone or iPad, add the app to the Home Screen first and open it from there.";
  }
  // Dev serves from Vite without the worker, so waiting for one would never end.
  if (!globals.production) return "This is a dev build with no service worker, so it cannot receive push.";
  return null;
}

/** The lock a remote device is under, in the words the switch shows. */
export function lockedMessage(hosted: boolean): string {
  return hosted
    ? "This device is locked, so it cannot turn notifications on yet. Unlock it with a passkey, then switch this on."
    : "This device is locked, and passkeys only work from the hosted client at coilyco.dev. Open it there, unlock this device, and switch this on.";
}

/** What a person reads when the browser, not the daemon, ends the subscribe. */
export function subscribeFailure(error: unknown): string {
  const name = error instanceof Error ? error.name : "";
  if (name === "NotAllowedError") return DENIED;
  if (name === "NotSupportedError") return "This browser cannot subscribe to push here.";
  return "The browser could not reach its push service, so nothing was turned on. Check the connection and try again.";
}

/** Base64url of the daemon's key as the bytes `subscribe` wants. */
export const applicationServerKey = (key: string): Uint8Array => new Uint8Array(fromBase64Url(key));

function sameKey(subscription: PushSubscription, key: string): boolean {
  const held = subscription.options?.applicationServerKey;
  if (!held) return false;
  const a = new Uint8Array(held);
  const b = applicationServerKey(key);
  return a.length === b.length && a.every((byte, index) => byte === b[index]);
}

/** The browser calls push needs, so the steps below run against fakes. */
export interface PushBrowser {
  permission(): NotificationPermission;
  requestPermission(): Promise<NotificationPermission>;
  existing(): Promise<PushSubscription | null>;
  subscribe(key: Uint8Array): Promise<PushSubscription>;
}

const WORKER_WAIT_MS = 5000;

export function liveBrowser(): PushBrowser {
  const registration = async (): Promise<ServiceWorkerRegistration> => {
    const timeout = new Promise<null>((resolve) => setTimeout(resolve, WORKER_WAIT_MS, null));
    const ready = await Promise.race([navigator.serviceWorker.ready, timeout]);
    if (!ready) throw new PushError(NO_WORKER);
    return ready;
  };
  return {
    permission: () => Notification.permission,
    requestPermission: () => Notification.requestPermission(),
    existing: async () => (await registration()).pushManager.getSubscription(),
    subscribe: async (key) => (await registration()).pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key as BufferSource }),
  };
}

/** Permission, then the key, then the browser, then the daemon. Permission goes first. */
export async function enablePush(channel: PushChannel, browser: PushBrowser, hosted: boolean): Promise<void> {
  let permission = browser.permission();
  if (permission === "default") permission = await browser.requestPermission();
  if (permission === "denied") throw new PushError(DENIED);
  if (permission !== "granted") throw new PushError(DISMISSED);

  try {
    const key = await channel.key();
    let subscription = await browser.existing();
    // A subscription is bound to one key, so another daemon's key needs a fresh one.
    if (subscription && !sameKey(subscription, key)) {
      await subscription.unsubscribe();
      subscription = null;
    }
    const created = subscription === null;
    if (!subscription) {
      try {
        subscription = await browser.subscribe(applicationServerKey(key));
      } catch (error) {
        throw new PushError(error instanceof PushError ? error.message : subscribeFailure(error));
      }
    }
    try {
      await channel.subscribe(subscription.toJSON());
    } catch (error) {
      // A browser subscription the daemon never heard of would only sit unused.
      if (created) await subscription.unsubscribe().catch(() => {});
      throw error;
    }
  } catch (error) {
    if (error instanceof Refusal && error.reason === "passkey_required") throw new PushError(lockedMessage(hosted));
    throw error instanceof Error ? new PushError(error.message) : error;
  }
}

/** Tells the daemon, then drops the browser's subscription even if it could not. */
export async function disablePush(channel: PushChannel, browser: PushBrowser): Promise<void> {
  const subscription = await browser.existing();
  if (!subscription) return;
  await channel.unsubscribe(subscription.endpoint).catch(() => {});
  await subscription.unsubscribe();
}

/** Daemon addresses this browser subscribed through. Malformed reads as none. */
export function parsePushHosts(raw: string | null): string[] {
  try {
    const parsed: unknown = JSON.parse(raw ?? "null");
    return Array.isArray(parsed) ? parsed.filter((each): each is string => typeof each === "string") : [];
  } catch {
    return [];
  }
}

export function loadPushHosts(): string[] {
  try {
    return parsePushHosts(localStorage.getItem(KEY));
  } catch {
    return [];
  }
}

export function storePushHosts(hosts: readonly string[]): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(hosts));
  } catch {
    // Storage can be off (a private window). The switch reads off after a reload.
  }
}

/** On only for a host this browser subscribed through and still holds a subscription. */
export function pushIsOn(subscribed: boolean, hosts: readonly string[], address: string | undefined): boolean {
  return subscribed && address !== undefined && hosts.includes(address);
}

/** The seat a tapped notification names, from the page address that opened the app. */
export function seatFromLocation(search: string): string | null {
  const seat = new URLSearchParams(search).get("seat");
  return seat || null;
}

export interface PendingSeat {
  session: string;
  /** Null until a host is attached, since the wait runs from when one could answer. */
  until: number | null;
}

/** A tapped seat opens once the attached host lists it, and is dropped after its wait. */
export function resolveSeat(pending: PendingSeat | null, attached: boolean, listed: readonly string[], now: number): { select: string | null; keep: PendingSeat | null } {
  if (!pending) return { select: null, keep: null };
  if (pending.until !== null && now > pending.until) return { select: null, keep: null };
  if (listed.includes(pending.session)) return { select: pending.session, keep: null };
  if (attached && pending.until === null) return { select: null, keep: { ...pending, until: now + SEAT_WAIT_MS } };
  return { select: null, keep: pending };
}

/** Takes a seat's notification down once the seat is open, since it has done its work. */
export async function closeSeatNotifications(session: string): Promise<void> {
  if (!("serviceWorker" in navigator)) return;
  try {
    const registration = await navigator.serviceWorker.getRegistration();
    for (const each of (await registration?.getNotifications({ tag: seatTag(session) })) ?? []) each.close();
  } catch {
    // A notification left standing is a nuisance, not a failure.
  }
}
