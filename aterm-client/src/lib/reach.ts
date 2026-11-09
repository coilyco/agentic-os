// Which layer kept a browser from a daemon, asked over HTTPS because a refused
// websocket shows a page nothing. See references/reach.md.

export type Layer = "ok" | "name" | "ownership" | "origin" | "answered" | "network";

export interface Diagnosis {
  layer: Layer;
  message: string;
}

const REACH_PATH = "/_aterm/reach";
const REFUSAL_HEADER = "X-Aterm-Refusal";
const NAMED: readonly string[] = ["name", "ownership", "origin"];

export type Ask = (url: string, init: RequestInit) => Promise<Response>;

/** What one HTTPS ask of the daemon came back as, before any wording. */
export type Reach =
  | { kind: "admitted"; ms: number }
  | { kind: "refused"; layer: "name" | "ownership" | "origin"; reason: string; status: number; ms: number }
  | { kind: "other"; status: number; ms: number }
  | { kind: "opaque"; ms: number }
  | { kind: "silent"; ms: number }
  | { kind: "failed"; ms: number };

/** The seams a test replaces. A page uses the defaults. */
export interface Io {
  ask: Ask;
  timeoutMs: number;
  now: () => number;
}

/** The daemon's HTTPS address behind a websocket address, or null when it is not one. */
export function daemonPage(address: string): URL | null {
  try {
    const page = new URL(address);
    if (page.protocol !== "wss:" && page.protocol !== "ws:") return null;
    page.protocol = page.protocol === "wss:" ? "https:" : "http:";
    page.pathname = "/";
    page.search = "";
    page.hash = "";
    return page;
  } catch {
    return null;
  }
}

const pageIo = (timeoutMs: number): Io => ({ ask: (url, init) => fetch(url, init), timeoutMs, now: () => performance.now() });

/** Asks the reach path over HTTPS, then unreadably if the first could not be read. */
export async function askReach(page: URL, io: Io): Promise<Reach> {
  const reach = new URL(REACH_PATH, page).toString();
  const started = io.now();
  const elapsed = () => Math.round(io.now() - started);
  const attempt = async (init: RequestInit): Promise<Response> => {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), io.timeoutMs);
    try {
      return await io.ask(reach, { ...init, cache: "no-store", signal: controller.signal });
    } finally {
      clearTimeout(timer);
    }
  };

  let failure: unknown;
  try {
    const response = await attempt({});
    if (response.ok) return { kind: "admitted", ms: elapsed() };
    const layer = response.headers.get(REFUSAL_HEADER) ?? "";
    const reason = (await response.text()).replace(/^aterm daemon:\s*/, "").trim();
    if (NAMED.includes(layer)) return { kind: "refused", layer: layer as "name" | "ownership" | "origin", reason, status: response.status, ms: elapsed() };
    return { kind: "other", status: response.status, ms: elapsed() };
  } catch (error) {
    // Unreadable: no answer, or an answer this page may not read.
    failure = error;
  }
  // A silent host is silent to the second ask too, so it is not asked.
  if (failure instanceof DOMException && failure.name === "AbortError") return { kind: "silent", ms: elapsed() };
  try {
    await attempt({ mode: "no-cors" });
    return { kind: "opaque", ms: elapsed() };
  } catch {
    return { kind: "failed", ms: elapsed() };
  }
}

export async function diagnose(address: string, ask: Ask = (url, init) => fetch(url, init), timeoutMs = 4000): Promise<Diagnosis> {
  const page = daemonPage(address);
  if (!page) return { layer: "network", message: `${address} is not a daemon address.` };
  const found = await askReach(page, { ...pageIo(timeoutMs), ask });

  switch (found.kind) {
    case "admitted":
      return { layer: "ok", message: "The daemon admitted this device, so the name, TLS, tailnet and ownership checks passed. Its session socket still did not open, and its log says why." };
    case "refused": {
      const lead = { name: "The daemon refused this address", ownership: "The daemon refused this device", origin: "The daemon refused this page's origin" }[found.layer];
      return { layer: found.layer, message: `${lead}: ${found.reason}. A retry cannot succeed until that changes.` };
    }
    case "other":
      return { layer: "answered", message: `The daemon answered HTTP ${found.status} but not as a refusal. It may be an older build.` };
    case "opaque":
      return {
        layer: "answered",
        message: `The daemon answered over HTTPS, so the name, TLS and tailnet route work. It would not let this page read why, which is how it refuses a device or this page's origin. Open ${page} in a browser tab and a refusal prints its reason.`,
      };
    case "silent":
    case "failed":
      return {
        layer: "network",
        message: `Nothing answered over HTTPS. That is the name not resolving (Private DNS or secure DNS can bypass MagicDNS), the tailnet route or Chrome's local network block, a TLS failure, or a stopped daemon. A browser cannot tell these apart. Open ${page} in a tab and Chrome's error code names which.`,
      };
  }
}
