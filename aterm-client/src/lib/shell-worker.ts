/** Registers the app-shell worker. Dev serves from Vite, so a cache would go stale. */
export function registerShellWorker(): void {
  if (!import.meta.env.PROD || !("serviceWorker" in navigator)) return;
  window.addEventListener("load", () => {
    navigator.serviceWorker.register(`${import.meta.env.BASE_URL}sw.js`).catch(() => {
      // Without it the client still runs. Only a launch with no daemon loses its shell.
    });
  });
}
