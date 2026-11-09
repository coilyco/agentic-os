// Which hop between this page and a daemon broke, from what a page can observe.
// A page sees no DNS, route or TLS of its own, so most hops are read from which
// request failed and how. references/reach.md says what is seen and what is not.
import { askReach, daemonPage, type Io, type Reach } from "./reach";

export type HopId = "resolve" | "route" | "tls" | "http" | "socket" | "admit";
/** `maybe`: this hop or a neighbour broke and a page cannot separate them. */
export type HopStatus = "pass" | "fail" | "maybe" | "skipped";
/** `seen`: the page observed it. `inferred`: read from another signal. */
export type Basis = "seen" | "inferred";

export interface Hop {
  id: HopId;
  title: string;
  status: HopStatus;
  basis: Basis;
  observed: string;
  /** One sentence on the likely cause, when the hop failed or may have. */
  cause: string;
}

export interface HopReport {
  address: string;
  checkedAt: string;
  ms: number;
  hops: Hop[];
  verdict: string;
  /** The daemon page, which a tab opens to get the browser's own error code. */
  page: string | null;
}

export const TITLES: Record<HopId, string> = {
  resolve: "Name resolves",
  route: "Tailnet route",
  tls: "TLS",
  http: "HTTP",
  socket: "Websocket upgrade",
  admit: "Daemon admits this device",
};
const ORDER: readonly HopId[] = ["resolve", "route", "tls", "http", "socket", "admit"];

/** What a page cannot see, said once on screen and again in the report. */
export const LIMITS: readonly string[] = [
  "A page never sees DNS, the route or TLS. A failed request reaches it as one error, whether the name did not resolve, the connection was refused or reset, or the certificate was wrong.",
  "It does see a request that hangs, which is how a host that is off the tailnet looks.",
  "A refused websocket upgrade shows no status or body, only close code 1006.",
  "Chrome's own error page names the exact failure. Open the daemon page in a tab to see it.",
];

type Socket =
  | { kind: "welcome"; ms: number }
  | { kind: "greeted-with-error"; error: string; ms: number }
  | { kind: "never-opened"; ms: number }
  | { kind: "closed"; code: number; reason: string; ms: number }
  | { kind: "no-welcome"; ms: number };

export interface SocketIo {
  open: (url: string) => WebSocket;
  timeoutMs: number;
  now: () => number;
}

const FORMAT = "aterm.daemon.v1";

/** Opens the daemon's socket, says hello, and reports how far it got. Never throws. */
export function probeSocket(address: string, io: SocketIo): Promise<Socket> {
  return new Promise((resolve) => {
    const started = io.now();
    const elapsed = () => Math.round(io.now() - started);
    let opened = false;
    let socket: WebSocket;
    const done = (result: Socket): void => {
      clearTimeout(timer);
      try {
        socket.close();
      } catch {
        // Already closed.
      }
      resolve(result);
    };
    const timer = setTimeout(() => done(opened ? { kind: "no-welcome", ms: elapsed() } : { kind: "never-opened", ms: elapsed() }), io.timeoutMs);
    try {
      socket = io.open(address);
    } catch {
      clearTimeout(timer);
      resolve({ kind: "never-opened", ms: 0 });
      return;
    }
    socket.addEventListener("open", () => {
      opened = true;
      socket.send(JSON.stringify({ type: "hello", format: FORMAT }));
    });
    socket.addEventListener("message", (event) => {
      try {
        const frame = JSON.parse(String(event.data)) as { type?: string; error?: string };
        if (frame.type !== "welcome") return;
        done(frame.error ? { kind: "greeted-with-error", error: frame.error, ms: elapsed() } : { kind: "welcome", ms: elapsed() });
      } catch {
        // Not a frame. The timeout reports it.
      }
    });
    // The error event carries nothing. The close that follows carries the code.
    socket.addEventListener("close", (event) => done(opened ? { kind: "closed", code: event.code, reason: event.reason, ms: elapsed() } : { kind: "never-opened", ms: elapsed() }));
  });
}

interface Seams {
  reach: Io;
  socket: SocketIo;
  /** The page's own scheme and host, for the one failure it knows before asking. */
  here: { protocol: string; hostname: string };
}

function withDefaults(seams: Partial<Seams>, timeoutMs: number): Seams {
  return {
    reach: seams.reach ?? { ask: (url, init) => fetch(url, init), timeoutMs, now: () => performance.now() },
    socket: seams.socket ?? { open: (url) => new WebSocket(url), timeoutMs, now: () => performance.now() },
    here: seams.here ?? { protocol: location.protocol, hostname: location.hostname },
  };
}

function blank(): Hop[] {
  return ORDER.map((id) => ({ id, title: TITLES[id], status: "skipped", basis: "inferred", observed: "Not tried, because an earlier hop failed.", cause: "" }));
}

function set(hops: Hop[], id: HopId, status: HopStatus, basis: Basis, observed: string, cause = ""): void {
  const hop = hops.find((each) => each.id === id)!;
  Object.assign(hop, { status, basis, observed, cause });
}

/** Every hop before `id` worked, since a later one answered. */
function passThrough(hops: Hop[], id: HopId): void {
  for (const each of ORDER.slice(0, ORDER.indexOf(id))) set(hops, each, "pass", "inferred", "Worked, since a later request got an answer.");
}

const seconds = (ms: number): string => `${Math.round(ms / 100) / 10} s`;

function fromReach(hops: Hop[], reach: Reach, timeoutMs: number): boolean {
  switch (reach.kind) {
    case "admitted":
      passThrough(hops, "http");
      set(hops, "http", "pass", "seen", `HTTP 200 in ${reach.ms} ms.`);
      return true;
    case "refused": {
      passThrough(hops, "http");
      set(hops, "http", "pass", "seen", `HTTP ${reach.status} in ${reach.ms} ms. The daemon answered.`);
      if (reach.layer === "origin") {
        set(hops, "socket", "fail", "seen", `The daemon refused the origin: ${reach.reason}`, "The daemon does not let this page open a session socket, so the page has to be one it allows.");
      } else {
        const cause = reach.layer === "name" ? "This page used an address that is not the daemon's own tailnet name, so use that name." : "This device is not one the daemon admits, so its tailnet login or tag has to change.";
        const what = reach.layer === "ownership" ? "device" : "address";
        set(hops, "admit", "fail", "seen", `The daemon refused this ${what}: ${reach.reason}`, cause);
      }
      return false;
    }
    case "other":
      passThrough(hops, "http");
      set(hops, "http", "fail", "seen", `HTTP ${reach.status} in ${reach.ms} ms, which is not the daemon's answer.`, "Something answered this address that is not an aterm daemon, or an older build of one.");
      return false;
    case "opaque":
      passThrough(hops, "http");
      set(hops, "http", "pass", "seen", `Answered in ${reach.ms} ms, but the daemon would not let this page read the status.`);
      return true;
    case "silent":
      set(hops, "resolve", "maybe", "inferred", "A page cannot see DNS.", "The name may not resolve, which a page cannot see.");
      set(hops, "route", "fail", "inferred", `No answer in ${seconds(timeoutMs)}, so nothing came back at all.`, "This device is probably off the tailnet, or the host is asleep or offline.");
      set(hops, "tls", "maybe", "inferred", "Not reached, or it hung.", "");
      return false;
    case "failed": {
      // Refused, reset, a bad certificate and an unresolved name are one error to a page.
      const seen = `The request failed in ${reach.ms} ms with no reason a page can read.`;
      set(hops, "resolve", "maybe", "inferred", seen, "The name may not resolve, so Tailscale may be off on this device or Private DNS may be bypassing MagicDNS.");
      set(hops, "route", "maybe", "inferred", "Same failure as hop 1.", "The connection may have been refused or reset, so the daemon or Tailscale on the host may be down.");
      set(hops, "tls", "maybe", "inferred", "Same failure as hop 1.", "The handshake or the certificate may have failed, as when a daemon started from a since-replaced binary resets every handshake.");
      return false;
    }
  }
}

function fromSocket(hops: Hop[], socket: Socket): void {
  switch (socket.kind) {
    case "welcome":
      set(hops, "socket", "pass", "seen", `Upgraded and open in ${socket.ms} ms.`);
      set(hops, "admit", "pass", "seen", "The daemon sent its welcome.");
      break;
    case "greeted-with-error":
      set(hops, "socket", "pass", "seen", "Upgraded and open.");
      set(hops, "admit", "fail", "seen", `The daemon's welcome said: ${socket.error}`, "The daemon refuses what this page asked, so the page and the daemon are probably different builds.");
      break;
    case "never-opened":
      set(hops, "socket", "fail", "seen", "The browser reported the upgrade failed, with close code 1006 and no status.", "The upgrade was refused or cut after HTTP worked, as when the daemon refuses the page's origin or restarts as it answers.");
      break;
    case "closed":
      set(hops, "socket", "pass", "seen", "Upgraded and open.");
      set(hops, "admit", "fail", "seen", `The daemon closed the socket with code ${socket.code}${socket.reason ? ` and said: ${socket.reason}` : " and gave no reason"}.`, "The daemon accepted the socket and then cut it before it greeted this page.");
      break;
    case "no-welcome":
      set(hops, "socket", "pass", "seen", "Upgraded and open.");
      set(hops, "admit", "fail", "seen", "The socket opened and the daemon never sent its welcome.", "The daemon may be stuck, or something other than an aterm daemon answers on this port.");
      break;
  }
}

function verdict(hops: Hop[]): string {
  const number = (hop: Hop) => ORDER.indexOf(hop.id) + 1;
  const failed = hops.find((hop) => hop.status === "fail");
  const maybe = hops.filter((hop) => hop.status === "maybe").map(number);
  const list = (numbers: number[]) => `hop ${numbers.slice(0, -1).join(", ")}${numbers.length > 1 ? " or " : ""}${numbers.at(-1)}`;
  if (failed) {
    const lead = failed.basis === "inferred" ? "Most likely broke at" : "Broke at";
    return `${lead} hop ${number(failed)}, ${failed.title}.${maybe.length ? ` A page cannot rule out ${list(maybe)}.` : ""}`;
  }
  return maybe.length ? `Broke at ${list(maybe)}. A page cannot tell which.` : "Every hop passed just now.";
}

/** Runs the six hops in order, stopping at the first that fails, and says which broke. */
export async function checkHops(address: string, seams: Partial<Seams> = {}, timeoutMs = 4000): Promise<HopReport> {
  const io = withDefaults(seams, timeoutMs);
  const started = io.reach.now();
  const hops = blank();
  const page = daemonPage(address);
  const done = (): HopReport => ({ address, checkedAt: new Date().toISOString(), ms: Math.round(io.reach.now() - started), hops, verdict: verdict(hops), page: page?.toString() ?? null });

  if (!page) {
    set(hops, "resolve", "fail", "seen", `${address} is not a websocket address.`, "The host address has to start with wss:// or ws://, so remove it and add the host again.");
    return done();
  }
  // The one failure known before asking: a secure page will not open a plain socket.
  const loopback = ["localhost", "127.0.0.1", "[::1]"].includes(page.hostname);
  if (io.here.protocol === "https:" && page.protocol === "http:" && !loopback) {
    set(hops, "tls", "fail", "seen", `This page is on HTTPS and the address is ${address}, a plain socket.`, "The browser refuses a plain socket from a secure page, so the host needs its wss:// address.");
    return done();
  }

  const reach = await askReach(page, io.reach);
  const answered = fromReach(hops, reach, timeoutMs);
  // An unreadable answer still gets the socket, which says more than the refusal.
  if (answered) {
    fromSocket(hops, await probeSocket(address, io.socket));
    if (reach.kind === "opaque" && hops.find((hop) => hop.id === "socket")!.status === "fail") {
      set(hops, "socket", "fail", "seen", "The browser reported the upgrade failed, with close code 1006 and no status.", "The daemon answered but would not let this page read it, which is how it refuses this page's origin or this device.");
    }
  }
  return done();
}

export interface ReportContext {
  label: string;
  page: string;
  agent: string;
  online: boolean | null;
  link: string;
}

/** The report as a message to prod-director, so no one describes it by hand. */
export function reportText(report: HopReport, context: ReportContext): string {
  const lines = [
    "To prod-director: aterm client connection report (COI-3256).",
    "",
    `Host: ${context.label}, ${report.address}`,
    `Page: ${context.page}`,
    `Browser: ${context.agent}`,
    `Device online per the browser: ${context.online === null ? "unknown" : context.online ? "yes" : "no"}. Link type: ${context.link || "unknown"}.`,
    `Checked: ${report.checkedAt}, took ${report.ms} ms.`,
    "",
    `Result: ${report.verdict}`,
    "",
    ...report.hops.flatMap((hop, index) => [
      `${index + 1}. ${hop.title} - ${{ pass: "passed", fail: "failed", maybe: "cannot tell", skipped: "not tried" }[hop.status]} (${hop.status === "skipped" ? "no signal" : hop.basis})`,
      `   Observed: ${hop.observed}`,
      ...(hop.cause ? [`   Likely cause: ${hop.cause}`] : []),
    ]),
    "",
    "Not visible from a page:",
    ...LIMITS.map((line) => `- ${line}`),
    ...(report.page ? ["", `Daemon page, which a browser tab opens with its own error code: ${report.page}`] : []),
  ];
  return lines.join("\n");
}
