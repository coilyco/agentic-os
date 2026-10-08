import { app, HOSTED, selectSession } from "./app.svelte";
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
      if (needsPasskey(app.typing)) throw new PushError(lockedMessage(HOSTED));
      await enablePush(channel, liveBrowser(), HOSTED);
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

/** A tapped notification names a seat, which opens once the host lists it. */
export function openSeatFromNotification(session: string): void {
  pending = { session, until: null };
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
  if (result.select) selectSession(result.select);
}

/** Takes the seat a tapped notification launched with out of the address. */
export function takeLaunchSeat(): void {
  const seat = seatFromLocation(location.search);
  if (!seat) return;
  history.replaceState(null, "", location.pathname + location.hash);
  openSeatFromNotification(seat);
}
