import { installState, isStandalone, type InstallPromptEvent } from "./install";

/** How long to wait for the browser's install prompt before pointing at its menu. */
const PROMPT_WAIT_MS = 2000;

export const pwa = $state({
  standalone: isStandalone((query) => window.matchMedia(query), (navigator as Navigator & { standalone?: boolean }).standalone),
  installedHere: false,
  prompt: null as InstallPromptEvent | null,
  waited: false,
  outcome: "" as "" | "accepted" | "dismissed",
});

window.addEventListener("beforeinstallprompt", (event) => {
  event.preventDefault();
  pwa.prompt = event as InstallPromptEvent;
});
window.addEventListener("appinstalled", () => {
  pwa.installedHere = true;
  pwa.prompt = null;
});
setTimeout(() => (pwa.waited = true), PROMPT_WAIT_MS);

export function currentInstallState() {
  return installState({
    standalone: pwa.standalone || pwa.installedHere,
    secure: window.isSecureContext,
    prompt: pwa.prompt !== null,
  });
}

export async function install(): Promise<void> {
  const offered = pwa.prompt;
  if (!offered) return;
  // A prompt can be used once, so a dismissal sends the person to the browser's own menu.
  pwa.prompt = null;
  await offered.prompt();
  pwa.outcome = (await offered.userChoice).outcome;
}
