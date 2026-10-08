// Hosts a daemon found on its tailnet (the `hosts` frame). Discovery only suggests
// addresses, so a found host that refuses this device shows as not answering.
import { FORMAT } from "./daemon-host";
import type { Host } from "./protocol";
import { parseHostInput } from "./saved-hosts";

export interface FoundHost {
  name: string;
  host: string;
  port: string;
  version?: string;
}

export const FOUND_PREFIX = "found:";

/** The reply's hosts, dropping any entry that is not a name and a port. */
export function parseFound(raw: unknown): FoundHost[] {
  if (!Array.isArray(raw)) return [];
  return raw.flatMap((entry): FoundHost[] => {
    const { name, host, port, version } = (entry ?? {}) as Record<string, unknown>;
    if (typeof host !== "string" || typeof port !== "string" || !host || !/^\d+$/.test(port)) return [];
    return [{ name: typeof name === "string" && name ? name : host, host, port, ...(typeof version === "string" ? { version } : {}) }];
  });
}

/** Hosts for found daemons this device does not list yet, by address. */
export function newFoundHosts(existing: readonly Host[], found: readonly FoundHost[]): Host[] {
  const known = new Set(existing.map((host) => host.address));
  const added: Host[] = [];
  for (const daemon of found) {
    let address: string;
    try {
      address = parseHostInput(`${daemon.host}:${daemon.port}`).address;
    } catch {
      continue;
    }
    if (known.has(address)) continue;
    known.add(address);
    added.push({ id: `${FOUND_PREFIX}${address}`, label: daemon.name, address, kind: "daemon", found: true, status: { kind: "checking" } });
  }
  return added;
}

interface HostsFrame {
  type: string;
  format?: string;
  error?: string;
  hosts?: unknown;
}

/** Asks the daemon at url which daemons it sees. Rejects when it cannot answer. */
export function discover(url: string, timeoutMs = 10000): Promise<FoundHost[]> {
  return new Promise((resolve, reject) => {
    const socket = new WebSocket(url);
    const timer = setTimeout(() => finish(new Error("no answer")), timeoutMs);
    function finish(result: FoundHost[] | Error): void {
      clearTimeout(timer);
      socket.close();
      if (result instanceof Error) reject(result);
      else resolve(result);
    }
    socket.addEventListener("open", () => socket.send(JSON.stringify({ type: "hello", format: FORMAT })));
    socket.addEventListener("error", () => finish(new Error("no answer")));
    socket.addEventListener("close", () => finish(new Error("closed")));
    socket.addEventListener("message", (event) => {
      const frame = JSON.parse(String(event.data)) as HostsFrame;
      if (frame.type === "welcome") {
        if (frame.error || frame.format !== FORMAT) finish(new Error(frame.error ?? "unsupported format"));
        else socket.send(JSON.stringify({ type: "hosts", id: "discover" }));
      } else if (frame.type === "hosts") finish(parseFound(frame.hosts));
      else if (frame.type === "error") finish(new Error(frame.error ?? "refused"));
    });
  });
}
