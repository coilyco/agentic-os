// Opens accounts' sign-in page in the system browser and hears it come back. The plugin's
// names: references/system-browser.md. Nothing logs the return, which holds a code.

import { HandoffError } from "./handoff";

export interface SystemBrowser {
  /** Opens accounts' sign-in address. Rejects in words when the plugin refuses it. */
  open(url: string): Promise<void>;
  /** The return the app was started with, once, then null. Only Android has one. */
  takeReturn(): Promise<string | null>;
  /** Calls `listener` with the address the app was opened with. Returns the stopper. */
  onReturn(listener: (url: string) => void): () => void;
  /** Calls `listener` when the browser was dismissed with no return. Android is late. */
  onClosed(listener: () => void): () => void;
}

type Invoke = (command: string, args?: Record<string, unknown>) => Promise<unknown>;
type Listen = (plugin: string, event: string, handler: (payload: unknown) => void) => Promise<{ unregister: () => Promise<void> }>;

const PLUGIN = "systembrowser";

function core(scope: unknown): { invoke: Invoke; listen: Listen } | null {
  const tauri = (scope as { __TAURI__?: { core?: { invoke?: unknown; addPluginListener?: unknown } } }).__TAURI__?.core;
  if (typeof tauri?.invoke !== "function" || typeof tauri.addPluginListener !== "function") return null;
  return { invoke: tauri.invoke as Invoke, listen: tauri.addPluginListener as Listen };
}

const addressOf = (payload: unknown): string | null => {
  const url = (payload as { url?: unknown } | null)?.url;
  return typeof url === "string" && url !== "" ? url : null;
};

/** The plugin's bridge, or null in a browser and in an app built without it. */
export function systemBrowser(scope: unknown = globalThis): SystemBrowser | null {
  const tauri = core(scope);
  if (!tauri) return null;

  // Registering is async and stopping is not, so an early stop must still unregister.
  const listen = (event: string, handler: (payload: unknown) => void): (() => void) => {
    let stopped = false;
    let unregister: (() => Promise<void>) | null = null;
    tauri
      .listen(PLUGIN, event, handler)
      .then((listener) => {
        unregister = listener.unregister;
        if (stopped) void unregister().catch(() => {});
      })
      .catch(() => {});
    return () => {
      stopped = true;
      if (unregister) void unregister().catch(() => {});
    };
  };

  return {
    async open(url) {
      try {
        await tauri.invoke(`plugin:${PLUGIN}|open`, { url });
      } catch (reason) {
        const why = typeof reason === "string" ? reason : reason instanceof Error ? reason.message : "";
        throw new HandoffError(`This app could not open the sign-in page${why ? `: ${why}` : "."}`);
      }
    },
    async takeReturn() {
      try {
        return addressOf({ url: await tauri.invoke(`plugin:${PLUGIN}|take_return`) });
      } catch {
        return null;
      }
    },
    onReturn(listener) {
      return listen("return", (payload) => {
        const url = addressOf(payload);
        if (url) listener(url);
      });
    },
    onClosed(listener) {
      return listen("closed", () => listener());
    },
  };
}
