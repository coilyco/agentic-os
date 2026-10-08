// The app shell for aterm. The build swaps the SHELL string below for the file list
// and a hash of their bytes (pwa-plugin.ts). It never touches a websocket, and every
// request that is not the shell goes to the network untouched. It also shows Web Push:
// .agents/skills/tooling-aterm-client/references/web-push.md
const SHELL = "__SHELL__";
const CACHE = `aterm-shell-${SHELL.version}`;
const SLOW_NETWORK_MS = 3000;

const inScope = (file) => new URL(file, self.registration.scope).href;

self.addEventListener("install", (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHE);
      for (const file of SHELL.files) {
        const response = await fetch(inScope(file), { cache: "reload" });
        // A sign-in redirect ending in a 200 would cache the sign-in page as the shell.
        if (!response.ok || response.redirected) throw new Error(`${file} did not come back as itself`);
        await cache.put(inScope(file), response);
      }
    })(),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      for (const name of await caches.keys()) {
        if (name.startsWith("aterm-shell-") && name !== CACHE) await caches.delete(name);
      }
      await self.clients.claim();
    })(),
  );
});

// A window that opens with the daemon down gets the shell, which then draws its own
// "not answering" state. With the daemon up, the network answers and wins.
async function openWindow(request) {
  const timeout = new Promise((resolve) => setTimeout(resolve, SLOW_NETWORK_MS, null));
  try {
    const network = fetch(request).catch(() => null);
    const live = await Promise.race([network, timeout]);
    if (live) return live;
  } catch {
    // fall through to the shell
  }
  const cache = await caches.open(CACHE);
  return (await cache.match(inScope("./"))) ?? fetch(request);
}

self.addEventListener("fetch", (event) => {
  const { request } = event;
  if (request.method !== "GET") return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin || !url.href.startsWith(self.registration.scope)) return;
  if (request.mode === "navigate") {
    event.respondWith(openWindow(request));
    return;
  }
  event.respondWith(
    caches.open(CACHE).then(async (cache) => (await cache.match(request, { ignoreSearch: true })) ?? fetch(request)),
  );
});

// A push that is not the daemon's payload still shows something, since showing nothing
// makes Chrome add its own notice.
function readPush(data) {
  let payload = null;
  try {
    payload = data ? data.json() : null;
  } catch {
    // Not JSON. The fallback below speaks for it.
  }
  const text = (value) => (typeof value === "string" && value ? value : "");
  const session = text(payload?.session);
  return {
    session,
    title: text(payload?.title) || (text(payload?.identity) ? `${payload.identity} is waiting` : "A seat is waiting"),
    body: text(payload?.body),
    tag: text(payload?.tag) || (session ? `aterm-${session}` : "aterm"),
  };
}

const windowsOf = () => self.clients.matchAll({ type: "window", includeUncontrolled: true });

// A visible, focused window already cues itself, so its notification is shown without a
// sound or buzz. It is still shown, and the tag replaces a repeat.
async function showPush(event) {
  const note = readPush(event.data);
  const watched = (await windowsOf()).some((client) => client.visibilityState === "visible" && client.focused);
  await self.registration.showNotification(note.title, {
    body: note.body,
    tag: note.tag,
    icon: inScope("icon-192.png"),
    data: { session: note.session },
    // silent and renotify cannot both be true.
    silent: watched,
    renotify: !watched,
  });
}

self.addEventListener("push", (event) => {
  event.waitUntil(showPush(event));
});

// A window already open is told which seat to show. With none, the app opens on it.
async function openSeat(session) {
  const open = (await windowsOf())[0];
  if (open) {
    try {
      await open.focus();
    } catch {
      // The browser may refuse to focus. The message still reaches the page.
    }
    open.postMessage({ type: "aterm-open-seat", session });
    return;
  }
  await self.clients.openWindow(session ? inScope(`./?seat=${encodeURIComponent(session)}`) : inScope("./"));
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(openSeat(event.notification.data?.session ?? ""));
});
