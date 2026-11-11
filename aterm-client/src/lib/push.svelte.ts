import { app, openByPerson, openSeat, placeNow, selectHost } from "./app.svelte";
import { chime } from "./chime";
import { askPermission, listenForTaps, nativeCore, permissionOf, showNative } from "./native-notify";
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

/** In the app, asks the system once per launch, from the foreground, where a prompt can
 * show. Later pings show only what was allowed, and a no is not asked again. */
export async function askNativeOnce(): Promise<void> {
  const core = nativeCore();
  if (!core || askedNative) return;
  askedNative = true;
  if ((await permissionOf(core)) === "ask") await askPermission(core);
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
