// The app's half of a notification with the app closed (COI-3752): the push plugin's FCM
// token, registered with the broker. A browser keeps Web Push (push.ts). The plugin's
// commands and the broker's shapes: the aterm-client skill's android-push reference.
import { BrokerError } from "./broker";
import { askPermission, permissionOf, rememberAsked, type Core } from "./native-notify";

/** Where the token goes: the broker's push app-register.v1 and app-unregister.v1. */
export interface AppRegistrar {
  register(token: string, device: string): Promise<void>;
  unregister(token: string): Promise<void>;
}

/** A tapped push. `session` is null for a machine coming back, which opens its list. */
export interface PushTap {
  kind: string;
  daemonId: string;
  session: string | null;
}

/** A failure already in words for the person. */
export class AppPushError extends Error {}

const KEY = "aterm.app-push.v1";

export const NOT_SET_UP = "Notifications with the app closed are not set up on this server. Notifications while the app is open still work.";
export const NOT_IN_THIS_APP = "This app cannot show notifications while it is closed. Notifications while it is open still work.";
export const SIGNED_OUT = "Sign in to your Treehouse account to turn this on.";
export const DENIED = "Notifications are off for Treehouse in this phone's settings. Allow them there, then switch this on again.";
const NO_TOKEN = "The phone's push service gave no token, so nothing was turned on. Check the connection and try again.";
const FAILED = "Notifications with the app closed could not be turned on. Try again.";

export function loadAppPushOn(): boolean {
  try {
    return localStorage.getItem(KEY) === "1";
  } catch {
    return false;
  }
}

export function storeAppPushOn(on: boolean): void {
  try {
    if (on) localStorage.setItem(KEY, "1");
    else localStorage.removeItem(KEY);
  } catch {
    // Storage can be off. The switch then reads off at the next launch.
  }
}

const codeOf = (error: unknown): string | undefined => {
  const code = (error as { code?: unknown } | null)?.code;
  return typeof code === "string" ? code : undefined;
};

/** What a person reads for a refusal from the plugin or the broker. `android` picks the
 * words for `push_off`: a phone build without the server's config, or the Mac app. */
export function refusal(error: unknown, android: boolean): string {
  if (error instanceof AppPushError) return error.message;
  const code = codeOf(error);
  if (code === "push_off") return android ? NOT_SET_UP : NOT_IN_THIS_APP;
  if (code === "no_token") return NO_TOKEN;
  if (code === "unauthorized") return SIGNED_OUT;
  // The broker writes its own words, a network failure included.
  if (error instanceof BrokerError && error.message) return error.message;
  return FAILED;
}

async function registered(core: Core): Promise<{ token: string; device: string }> {
  const reply = (await core.invoke("plugin:push|register")) as { token?: unknown; device?: unknown } | null;
  if (typeof reply?.token !== "string" || !reply.token) throw Object.assign(new Error("no token"), { code: "no_token" });
  // The broker takes 1 to 64 characters, and the plugin already cuts to 64.
  const device = typeof reply.device === "string" && reply.device ? reply.device.slice(0, 64) : "Android";
  return { token: reply.token, device };
}

/** Asks for the system's permission if it was never asked, then registers the token. */
export async function enableAppPush(core: Core, registrar: AppRegistrar): Promise<void> {
  const permission = await permissionOf(core);
  if (permission === "denied") throw new AppPushError(DENIED);
  if (permission === "ask") {
    rememberAsked();
    if (!(await askPermission(core))) throw new AppPushError(DENIED);
  }
  const { token, device } = await registered(core);
  await registrar.register(token, device);
}

/** Drops this phone's token at the broker. No token means nothing was ever registered. */
export async function disableAppPush(core: Core, registrar: AppRegistrar): Promise<void> {
  const token = await core.invoke("plugin:push|token");
  if (typeof token === "string" && token) await registrar.unregister(token);
}

/** At every sign-in with the switch on: the broker wants the token on each launch. */
export async function reRegister(core: Core, registrar: AppRegistrar): Promise<boolean> {
  try {
    const { token, device } = await registered(core);
    await registrar.register(token, device);
    return true;
  } catch {
    return false;
  }
}

/** The plugin's tap, or null for anything that is not one. */
export function parseTap(value: unknown): PushTap | null {
  const tap = value as { kind?: unknown; daemon_id?: unknown; session?: unknown } | null;
  if (!tap || typeof tap.daemon_id !== "string" || !tap.daemon_id) return null;
  const session = typeof tap.session === "string" && tap.session ? tap.session : null;
  return { kind: typeof tap.kind === "string" ? tap.kind : "", daemonId: tap.daemon_id, session };
}

/** The tap that started or woke the app, once. */
export async function takeTap(core: Core): Promise<PushTap | null> {
  try {
    return parseTap(await core.invoke("plugin:push|take_tap"));
  } catch {
    return null;
  }
}

/** A plugin event for as long as the page lives. Returns how to stop. */
async function listen(core: Core, event: string, callback: (payload: unknown) => void): Promise<() => void> {
  if (!core.addPluginListener) return () => {};
  try {
    const listener = await core.addPluginListener("push", event, callback);
    return () => void listener.unregister();
  } catch {
    return () => {};
  }
}

/** A tap on a push while the app is running. */
export function listenForPushTaps(core: Core, onTap: (tap: PushTap) => void): Promise<() => void> {
  return listen(core, "tap", (payload) => {
    const tap = parseTap(payload);
    if (tap) onTap(tap);
  });
}

/** FCM rotated the token, so the broker needs the new one. */
export function listenForTokenRefresh(core: Core, onRefresh: () => void): Promise<() => void> {
  return listen(core, "tokenRefresh", () => onRefresh());
}
