// aterm.daemon.v1 over the daemon's loopback websocket. The frame reference is
// the Wire contract section of the aterm daemon page in coilyco/agentic-os.
import { parseFrom } from "./messages";
import type { Ask, AskOutcome, HostConnection, HostEvent, MessageState, PeerMessage, Session } from "./protocol";
import { parseRoster } from "./roster";

export const FORMAT = "aterm.daemon.v1";
const LOOPBACK_DAEMON = "ws://127.0.0.1:7419";

// The dev server reaches the local daemon. A build is served by the daemon
// itself, so it dials the origin it came from.
export function daemonUrl(page: Pick<Location, "protocol" | "host">, dev: boolean, override?: string): string {
  if (override) return override;
  if (dev || page.protocol === "file:") return LOOPBACK_DAEMON;
  return `${page.protocol === "https:" ? "wss" : "ws"}://${page.host}/`;
}

/** The daemon's machine by name, or "this Mac" when it is the machine you are on. */
export function hostLabel(page: Pick<Location, "hostname">, dev: boolean): string {
  const name = page.hostname;
  if (dev || !name || name === "localhost" || name === "127.0.0.1" || name === "[::1]") return "this Mac";
  return name.split(".")[0] ?? name;
}

export const DEFAULT_DAEMON_URL =
  typeof location === "undefined" ? LOOPBACK_DAEMON : daemonUrl(location, import.meta.env.DEV, import.meta.env.VITE_ATERM_DAEMON_WS);

interface SessionView {
  name: string;
  role: string;
  identity: string;
  seat: string;
  ready: boolean;
  bracketed_paste: boolean;
  kai_drafting: boolean;
  pending: number;
}

interface MessageView {
  id: string;
  from: string;
  target: string;
  session?: string;
  state: MessageState;
  reason?: string;
}

// ask_choice as the daemon sends it (coilyco/agentic-os#1724).
interface AskView {
  id: string;
  session: string;
  from?: string;
  header?: string;
  question: string;
  options: { label: string; description?: string }[];
  allow_other?: boolean;
  multi?: boolean;
}

export function toAsk(view: AskView): Ask {
  return {
    id: view.id,
    session: view.session,
    header: view.header ?? "",
    question: view.question,
    options: view.options.map((option) => ({ label: option.label, description: option.description ?? "" })),
    allowOther: view.allow_other ?? false,
    multi: view.multi ?? false,
  };
}

interface Frame {
  type: string;
  id?: string;
  format?: string;
  session?: string;
  data?: string;
  sessions?: SessionView[];
  message?: MessageView;
  roster?: unknown;
  role?: string;
  seat?: string;
  code?: number;
  error?: string;
  ask?: AskView;
  ask_id?: string;
  state?: string;
}

/** Output this recent means the seat is busy. A working agent redraws constantly. */
export const BUSY_WINDOW_MS = 1500;
/** Echo, replay, and redraw this client caused are not the agent working. */
const SELF_WINDOW_MS = 800;

export function toSession(view: SessionView, busy = false): Session {
  return {
    id: view.name,
    role: view.role,
    identity: view.identity,
    seat: view.seat,
    state: view.ready && !busy ? "idle" : "working",
    pending: view.pending,
    drafting: view.kai_drafting,
    paste: view.bracketed_paste,
  };
}

export function toMessage(view: MessageView): PeerMessage {
  return {
    id: view.id,
    from: parseFrom(view.from),
    target: view.target,
    session: view.session ?? null,
    state: view.state,
    reason: view.reason ?? null,
  };
}

function encode(text: string): string {
  const bytes = new TextEncoder().encode(text);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function decode(base64: string): Uint8Array {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index);
  return bytes;
}

/** What a person reads when the daemon refuses a launch, keyed by its exit code. */
export function launchRefusal(code: number | undefined, error: string | undefined): string {
  if (code === 2) return `The daemon would not launch that seat: ${error ?? "unknown role or seat"}.`;
  if (code === 5) return `The launch failed on the host: ${error ?? "no reason given"}.`;
  return error ?? "The daemon refused a request.";
}

export class DaemonHost implements HostConnection {
  readonly canLaunch = true;
  private launches = new Map<string, string>();
  private socket: WebSocket;
  private listeners = new Set<(event: HostEvent) => void>();
  private outbox: string[] = [];
  private open = false;
  private nextId = 0;
  private last: { sessions?: Session[]; roles?: HostEvent } = {};
  private views: SessionView[] = [];
  private refs = new Map<string, number>();
  private lastOutput = new Map<string, number>();
  private lastPoke = new Map<string, number>();
  private busy = new Set<string>();
  private ticker: ReturnType<typeof setInterval>;

  constructor(url = DEFAULT_DAEMON_URL) {
    this.socket = new WebSocket(url);
    this.socket.addEventListener("open", () => this.socket.send(JSON.stringify({ type: "hello", format: FORMAT })));
    this.socket.addEventListener("message", (event) => this.receive(JSON.parse(String(event.data)) as Frame));
    this.socket.addEventListener("close", () => this.emit({ type: "closed", reason: "The daemon closed the connection." }));
    this.ticker = setInterval(() => this.settle(), 500);
  }

  subscribe(listener: (event: HostEvent) => void): () => void {
    this.listeners.add(listener);
    if (this.last.roles) listener(this.last.roles);
    if (this.last.sessions) listener({ type: "sessions", sessions: this.last.sessions });
    return () => this.listeners.delete(listener);
  }

  attach(sessionId: string, rows: number, cols: number): void {
    this.refs.set(sessionId, (this.refs.get(sessionId) ?? 0) + 1);
    this.lastPoke.set(sessionId, Date.now());
    this.request({ type: "attach", session: sessionId, replay: true, rows, cols });
  }

  // The activity monitor holds its own reference, so closing a terminal tab
  // leaves the seat watched and the daemon never sees a detach for it.
  detach(sessionId: string): void {
    const left = (this.refs.get(sessionId) ?? 1) - 1;
    if (left > 0) {
      this.refs.set(sessionId, left);
      return;
    }
    this.refs.delete(sessionId);
    this.request({ type: "detach", session: sessionId });
  }

  input(sessionId: string, data: string): void {
    this.lastPoke.set(sessionId, Date.now());
    this.request({ type: "input", session: sessionId, data: encode(data) });
  }

  resize(sessionId: string, rows: number, cols: number): void {
    this.lastPoke.set(sessionId, Date.now());
    this.request({ type: "resize", session: sessionId, rows, cols });
  }

  answer(askId: string, picks: number[], text?: string): void {
    this.request({ type: "answer", ask_id: askId, picks, ...(text ? { text } : {}) });
  }

  cancelAsk(askId: string): void {
    this.request({ type: "cancel_ask", ask_id: askId });
  }

  launch(role: string, seat: string): void {
    const id = this.request({ type: "launch", role, seat });
    this.launches.set(id, role);
    this.emit({ type: "launch", role, state: "starting", text: `Opening ${role} on ${seat}. Its window opens on the host.` });
  }

  close(): void {
    clearInterval(this.ticker);
    this.listeners.clear();
    this.socket.close();
  }

  /** Watch every live seat without replay or resize, to see when it is busy. */
  private monitor(): void {
    const live = new Set(this.views.map((view) => view.name));
    for (const name of live) {
      if (this.refs.has(name)) continue;
      this.refs.set(name, 1);
      this.request({ type: "attach", session: name, replay: false });
    }
    for (const name of [...this.refs.keys()]) {
      if (!live.has(name)) {
        this.refs.delete(name);
        this.busy.delete(name);
      }
    }
  }

  private settle(): void {
    const now = Date.now();
    let changed = false;
    for (const name of [...this.busy]) {
      if (now - (this.lastOutput.get(name) ?? 0) > BUSY_WINDOW_MS) {
        this.busy.delete(name);
        changed = true;
      }
    }
    if (changed) this.publishSessions();
  }

  private noteOutput(name: string): void {
    const now = Date.now();
    if (now - (this.lastPoke.get(name) ?? 0) < SELF_WINDOW_MS) return;
    this.lastOutput.set(name, now);
    if (!this.busy.has(name)) {
      this.busy.add(name);
      this.publishSessions();
    }
  }

  private publishSessions(): void {
    this.last.sessions = this.views.map((view) => toSession(view, this.busy.has(view.name)));
    this.emit({ type: "sessions", sessions: this.last.sessions });
  }

  private request(frame: Record<string, unknown>): string {
    const id = `c${++this.nextId}`;
    const line = JSON.stringify({ id, ...frame });
    if (this.open) this.socket.send(line);
    else this.outbox.push(line);
    return id;
  }

  private receive(frame: Frame): void {
    switch (frame.type) {
      case "welcome":
        if (frame.error || frame.format !== FORMAT) {
          this.emit({ type: "closed", reason: frame.error ?? `The daemon speaks ${frame.format}.` });
          this.socket.close();
          return;
        }
        this.open = true;
        this.request({ type: "subscribe", channel: "sessions" });
        this.request({ type: "roster" });
        for (const line of this.outbox.splice(0)) this.socket.send(line);
        return;
      case "ask":
        if (frame.ask) this.emit({ type: "ask", ask: toAsk(frame.ask) });
        return;
      case "asked":
        if (frame.ask_id) this.emit({ type: "asked", id: frame.ask_id, outcome: (frame.state ?? "answered") as AskOutcome });
        return;
      case "sessions":
        this.views = frame.sessions ?? [];
        this.monitor();
        this.publishSessions();
        return;
      case "roster":
        this.last.roles = { type: "roster", roles: parseRoster(frame.roster) };
        this.emit(this.last.roles);
        return;
      case "output":
        if (!frame.session || !frame.data) return;
        this.noteOutput(frame.session);
        this.emit({ type: "output", sessionId: frame.session, data: decode(frame.data) });
        return;
      case "message":
        if (frame.message) this.emit({ type: "message", message: toMessage(frame.message) });
        return;
      case "launched":
        if (frame.id) this.launches.delete(frame.id);
        this.emit({ type: "launch", role: frame.role ?? "", state: "started", text: `${frame.role} launched. It appears here once its session starts.` });
        return;
      case "error": {
        // Code 2: the answer does not fit the ask. Code 3: the ask is gone or settled.
        const role = frame.id ? this.launches.get(frame.id) : undefined;
        if (role !== undefined && frame.id) {
          this.launches.delete(frame.id);
          this.emit({ type: "launch", role, state: "failed", text: launchRefusal(frame.code, frame.error) });
        } else {
          this.emit({ type: "notice", text: frame.error ?? "The daemon refused a request." });
        }
        return;
      }
    }
  }

  private emit(event: HostEvent): void {
    for (const listener of this.listeners) listener(event);
  }
}

/** One round trip to learn whether a daemon answers and how many sessions it holds. */
export function probe(url = DEFAULT_DAEMON_URL, timeoutMs = 2000): Promise<number> {
  return new Promise((resolve, reject) => {
    const socket = new WebSocket(url);
    const timer = setTimeout(() => finish(new Error("no answer")), timeoutMs);
    function finish(result: number | Error): void {
      clearTimeout(timer);
      socket.close();
      if (result instanceof Error) reject(result);
      else resolve(result);
    }
    socket.addEventListener("open", () => socket.send(JSON.stringify({ type: "hello", format: FORMAT })));
    socket.addEventListener("error", () => finish(new Error("no answer")));
    socket.addEventListener("message", (event) => {
      const frame = JSON.parse(String(event.data)) as Frame;
      if (frame.type === "welcome") socket.send(JSON.stringify({ type: "list", id: "probe" }));
      else if (frame.type === "sessions") finish(frame.sessions?.length ?? 0);
      else if (frame.type === "error") finish(new Error(frame.error ?? "refused"));
    });
  });
}
