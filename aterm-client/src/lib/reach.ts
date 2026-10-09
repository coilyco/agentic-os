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

type Ask = (url: string, init: RequestInit) => Promise<Response>;

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

export async function diagnose(address: string, ask: Ask = (url, init) => fetch(url, init), timeoutMs = 4000): Promise<Diagnosis> {
  const page = daemonPage(address);
  if (!page) return { layer: "network", message: `${address} is not a daemon address.` };
  const reach = new URL(REACH_PATH, page).toString();
  const attempt = async (init: RequestInit): Promise<Response> => {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    try {
      return await ask(reach, { ...init, cache: "no-store", signal: controller.signal });
    } finally {
      clearTimeout(timer);
    }
  };

  try {
    const response = await attempt({});
    if (response.ok) {
      return { layer: "ok", message: "The daemon admitted this device, so the name, TLS, tailnet and ownership checks passed. Its session socket still did not open, and its log says why." };
    }
    const layer = response.headers.get(REFUSAL_HEADER) ?? "";
    const reason = (await response.text()).replace(/^aterm daemon:\s*/, "").trim();
    if (NAMED.includes(layer)) {
      const lead = { name: "The daemon refused this address", ownership: "The daemon refused this device", origin: "The daemon refused this page's origin" }[layer as "name" | "ownership" | "origin"];
      return { layer: layer as Layer, message: `${lead}: ${reason}. A retry cannot succeed until that changes.` };
    }
    return { layer: "answered", message: `The daemon answered HTTP ${response.status} but not as a refusal. It may be an older build.` };
  } catch {
    // Unreadable: no answer, or an answer this page may not read.
  }

  try {
    await attempt({ mode: "no-cors" });
    return {
      layer: "answered",
      message: `The daemon answered over HTTPS, so the name, TLS and tailnet route work. It would not let this page read why, which is how it refuses a device or this page's origin. Open ${page} in a browser tab and a refusal prints its reason.`,
    };
  } catch {
    return {
      layer: "network",
      message: `Nothing answered over HTTPS. That is the name not resolving (Private DNS or secure DNS can bypass MagicDNS), the tailnet route or Chrome's local network block, a TLS failure, or a stopped daemon. A browser cannot tell these apart. Open ${page} in a tab and Chrome's error code names which.`,
    };
  }
}
