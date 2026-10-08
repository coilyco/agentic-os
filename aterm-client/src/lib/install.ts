/** Chrome's install prompt, held until asked for. Not in the DOM lib, so typed here. */
export interface InstallPromptEvent extends Event {
  prompt(): Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
}

export type InstallState =
  | { kind: "installed" }
  /** The browser offered its own prompt, so one tap installs. */
  | { kind: "ready" }
  /** Secure, but no prompt: Safari installs from a menu, Chrome waits for some use. */
  | { kind: "menu" }
  /** Plain http from anywhere but this machine cannot be installed at all. */
  | { kind: "insecure" };

export interface InstallEnvironment {
  standalone: boolean;
  secure: boolean;
  prompt: boolean;
}

export function installState(env: InstallEnvironment): InstallState {
  if (env.standalone) return { kind: "installed" };
  if (!env.secure) return { kind: "insecure" };
  return { kind: env.prompt ? "ready" : "menu" };
}

/** The display modes a manifest installs into. All of them hide the browser's tabs. */
export function isStandalone(media: (query: string) => { matches: boolean }, navigatorStandalone: boolean | undefined): boolean {
  return navigatorStandalone === true || ["standalone", "minimal-ui", "fullscreen", "window-controls-overlay"].some((mode) => media(`(display-mode: ${mode})`).matches);
}
