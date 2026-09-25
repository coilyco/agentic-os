// Hosts added by hand on this device. Tailnet names are opaque, so the hosted
// build ships none and each device remembers its own (teable:coilyco/website#8256).
const KEY = "aterm.hosts.v1";
const DAEMON_PORT = 7419;
const IPV4 = /^\d{1,3}(\.\d{1,3}){3}$/;

function labelFor(hostname: string): string {
  return IPV4.test(hostname) ? hostname : (hostname.split(".")[0] ?? hostname);
}

export interface SavedHost {
  label: string;
  address: string;
}

/** A tailnet name, name:port, or ws(s) URL, as a host. Throws a readable message. */
export function parseHostInput(input: string): SavedHost {
  const text = input.trim();
  if (!text) throw new Error("Paste the host's tailnet name.");
  if (/^wss?:\/\//.test(text)) {
    const url = new URL(text);
    return { label: labelFor(url.hostname), address: url.href };
  }
  if (/[\s/]/.test(text)) throw new Error("That is not a host name. Paste just the name, like machine.tailnet.ts.net.");
  const [name = "", port] = text.split(":");
  if (IPV4.test(name)) throw new Error("Use the tailnet name, not an IP address. The daemon only answers by name.");
  if (port !== undefined && !/^\d+$/.test(port)) throw new Error("The port after the colon should be a number.");
  return { label: labelFor(name), address: `wss://${name}:${port ?? DAEMON_PORT}/` };
}

export function loadSavedHosts(): SavedHost[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(KEY) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((host): host is SavedHost => typeof host?.address === "string" && typeof host?.label === "string") : [];
  } catch {
    return [];
  }
}

export function storeSavedHosts(hosts: readonly SavedHost[]): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(hosts));
  } catch {
    // Storage can be off (a private window). The host still works until reload.
  }
}
