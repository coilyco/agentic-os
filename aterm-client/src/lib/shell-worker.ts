import { IN_APP } from "./build";

/** Registers the app-shell worker. Dev serves from Vite, so a cache would go stale. */
export function registerShellWorker(): void {
  // The app ships its shell in the APK, and a cached one would outlive an update.
  if (!import.meta.env.PROD || IN_APP || !("serviceWorker" in navigator)) return;
  window.addEventListener("load", () => {
    navigator.serviceWorker.register(`${import.meta.env.BASE_URL}sw.js`).catch(() => {
      // Without it the client still runs. Only a launch with no daemon loses its shell.
    });
  });
}
