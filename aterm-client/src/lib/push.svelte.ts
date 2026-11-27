import { app, appRegistrar, beforeSignOut, brokeredHostOf, openByPerson, openSeat, placeNow, selectHost } from "./app.svelte";
import { disableAppPush, enableAppPush, listenForPushTaps, listenForTokenRefresh, loadAppPushOn, reRegister, refusal, SIGNED_OUT, storeAppPushOn, takeTap, type PushTap } from "./app-push";
import { chime } from "./chime";
import { askedBefore, askPermission, listenForTaps, nativeCore, permissionOf, rememberAsked, showNative } from "./native-notify";
import { noticeFor, SeenIds, wantsDesktopNotice } from "./notify";
import type { Notify } from "./protocol";
import { disablePush, enablePush, liveBrowser, loadPushHosts, lockedMessage, PushError, pushIsOn, pushUnavailable, resolveSeat, seatFromLocation, storePushHosts, type PendingSeat } from "./push";
import { needsPasskey } from "./typing";

/** The Web Push switch: reads on, a call in flight, and why the last one failed. */
export const push = $state({ on: false, busy: false, message: "", failed: false });

let hosts = loadPushHosts();
let pending: PendingSeat | null = null;

const attachedAddress = () => app.hosts.find((host) => host.id === app.attachedHostId)?.address;

/** Why the switch cannot work on this page at all, shown beside it. Null when it can. */
export const pushBlocked = () => pushUnavailable();

/** Reads the browser's subscription for the attached host, on attach and reconnect. */
export async function syncPush(): Promise<void> {
  if (!app.webPush || pushBlocked()) {
    push.on = false;
    return;
  }
  try {
    push.on = pushIsOn((await liveBrowser().existing()) !== null, hosts, attachedAddress());
  } catch {
    push.on = false;
  }
}

/** Flips the switch. A failure leaves it where it was and says why. */
export async function setPush(want: boolean): Promise<void> {
  const channel = app.connection?.push;
  const address = attachedAddress();
  if (!channel || !address || push.busy) return;
  push.busy = true;
  push.message = "";
  push.failed = false;
  try {
    if (want) {
      if (needsPasskey(app.typing)) throw new PushError(lockedMessage(placeNow()));
      await enablePush(channel, liveBrowser(), placeNow());
      hosts = [...new Set([...hosts, address])];
    } else {
      await disablePush(channel, liveBrowser());
      hosts = hosts.filter((each) => each !== address);
    }
    storePushHosts(hosts);
    push.on = want;
  } catch (error) {
    push.failed = true;
    push.message = error instanceof Error ? error.message : String(error);
  } finally {
    push.busy = false;
  }
}

/** The seat is on a host this browser subscribed through, so reach it. */
function reachPushHost(): void {
  const current = attachedAddress();
  if (current !== undefined && hosts.includes(current)) return;
  const host = app.hosts.find((each) => hosts.includes(each.address));
  if (!host) return;
  // A host still being probed attaches itself when it answers.
  if (host.status.kind === "online") selectHost(host);
  else app.selectedHostId = host.id;
}

/** A tapped notification names a seat, which opens once the host lists it. */
export function openSeatFromNotification(session: string): void {
  pending = { session, until: null };
  reachPushHost();
  settleSeat();
}

/** Run when the attached host or its sessions change. */
export function settleSeat(): void {
  const result = resolveSeat(
    pending,
    app.attachedHostId !== null,
    app.sessions.map((session) => session.id),
    Date.now(),
  );
  pending = result.keep;
  // The tap is the click. It waits SEAT_WAIT_MS for the seat to be listed.
  if (result.select) openByPerson(result.select);
}

/** Takes the seat a tapped notification launched with out of the address. */
export function takeLaunchSeat(): void {
  const seat = seatFromLocation(location.search);
  if (!seat) return;
  history.replaceState(null, "", location.pathname + location.hash);
  openSeatFromNotification(seat);
}

const seenNotes = new SeenIds();

let askedNative = false;

/** In the app, asks the system once ever, from the foreground, where a prompt can show.
 * Later pings show only what was allowed, and a no is remembered, not asked again. */
export async function askNativeOnce(): Promise<void> {
  const core = nativeCore();
  if (!core || askedNative || askedBefore()) return;
  askedNative = true;
  if ((await permissionOf(core)) !== "ask") return;
  // Marked before the dialog, so a kill with it showing still counts as asked.
  rememberAsked();
  await askPermission(core);
}

/** In the app, a tap on a notification opens the seat it names. Returns how to stop. */
export async function listenForNativeTaps(): Promise<() => void> {
  const core = nativeCore();
  return core ? listenForTaps(core, openSeatFromNotification) : () => {};
}

/** A coordinator ping: the chime if Sound is on, and a notice if the browser allows.
 * The tap opens the sending seat, as a Web Push does. Each id is handled once. */
export async function handleNotify(note: Notify): Promise<void> {
  if (!seenNotes.first(note.id)) return;
  if (app.alerts.sound) chime();
  const core = nativeCore();
  if (core) {
    // The app shows it through the system, only while the person is not on that seat.
    const seen = { focused: typeof document !== "undefined" && document.hasFocus(), openSeat: openSeat() };
    if (wantsDesktopNotice(seen, note) && (await permissionOf(core)) === "granted") await showNative(core, note);
    return;
  }
  if (typeof Notification === "undefined" || Notification.permission !== "granted") return;
  if (!wantsDesktopNotice({ focused: typeof document !== "undefined" && document.hasFocus(), openSeat: openSeat() }, note)) return;
  const { title, options } = noticeFor(note);
  try {
    const registration = await navigator.serviceWorker?.getRegistration();
    if (registration) {
      await registration.showNotification(title, options);
      return;
    }
    const shown = new Notification(title, options);
    shown.onclick = () => {
      window.focus();
      openSeatFromNotification(note.session);
    };
  } catch {
    // A refused notice leaves the chime, which already played.
  }
}

// --- Push with the app closed, in the Mac and Android apps (COI-3752) ---

/** The app's switch: the push plugin's token, registered with the broker. */
export const appPush = $state({ on: loadAppPushOn(), busy: false, message: "", failed: false });

const android = () => typeof navigator !== "undefined" && /Android/i.test(navigator.userAgent);

/** True in the app with a Treehouse account surface. A browser keeps Web Push. */
export const appPushShown = (): boolean => nativeCore() !== null && appRegistrar !== null && app.accounts.configured;

/** Why the switch cannot turn on now, shown beside it. Null when it can. */
export const appPushBlocked = (): string | null => (app.accounts.status === "signed-in" ? null : SIGNED_OUT);

/** Flips the app's switch. A failure leaves it where it was and says why. */
export async function setAppPush(want: boolean): Promise<void> {
  const core = nativeCore();
  if (!core || !appRegistrar || appPush.busy) return;
  appPush.busy = true;
  appPush.message = "";
  appPush.failed = false;
  try {
    if (want) await enableAppPush(core, appRegistrar);
    // Signed out, sign-out already dropped the token, so off is only kept here.
    else if (app.accounts.status === "signed-in") await disableAppPush(core, appRegistrar);
    appPush.on = want;
    storeAppPushOn(want);
  } catch (error) {
    appPush.failed = true;
    appPush.message = refusal(error, android());
  } finally {
    appPush.busy = false;
  }
}

/** On each sign-in with the switch on, the broker gets this launch's token. */
export async function registerOnSignIn(): Promise<void> {
  const core = nativeCore();
  if (!core || !appRegistrar || !appPush.on || app.accounts.status !== "signed-in") return;
  await reRegister(core, appRegistrar);
}

let pendingTap: PushTap | null = null;

/** A tapped push names a machine, which is listed only once the account is signed in. */
export function openPushTap(tap: PushTap): void {
  pendingTap = tap;
  settlePushTap();
}

/** Run when the host list changes: the machine is listed, so open it, then its seat. */
export function settlePushTap(): void {
  const tap = pendingTap;
  const host = tap ? brokeredHostOf(tap.daemonId) : undefined;
  if (!tap || !host) return;
  pendingTap = null;
  if (app.attachedHostId !== host.id) {
    if (host.status.kind === "online") selectHost(host);
    else app.selectedHostId = host.id;
  }
  // Not openSeatFromNotification: its Web Push host would win over the tapped machine.
  if (tap.session) {
    pending = { session: tap.session, until: null };
    settleSeat();
  }
}

/** The app's push: a tap that launched it, taps while it runs, and a rotated token. */
export async function startAppPush(): Promise<() => void> {
  const core = nativeCore();
  if (!core) return () => {};
  beforeSignOut(async () => {
    if (appRegistrar && appPush.on) await disableAppPush(core, appRegistrar).catch(() => undefined);
  });
  const launched = await takeTap(core);
  if (launched) openPushTap(launched);
  const stopTaps = await listenForPushTaps(core, openPushTap);
  const stopRefresh = await listenForTokenRefresh(core, () => void registerOnSignIn());
  return () => {
    stopTaps();
    stopRefresh();
  };
}
