// aterm.daemon.v1 over the daemon's loopback websocket. The frame reference is
// the Wire contract section of the aterm daemon page in coilyco/agentic-os.
import { parseFrom } from "./messages";
import type { HostConnection, HostEvent, MessageState, PeerMessage, Session } from "./protocol";
import { parseRoster } from "./roster";

export const FORMAT = "aterm.daemon.v1";
export const DEFAULT_DAEMON_URL = import.meta.env.VITE_ATERM_DAEMON_WS ?? "ws://127.0.0.1:7419";

interface SessionView {
  name: string;
  role: string;
  identity: string;
  seat: string;
  ready: boolean;
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
}

export function toSession(view: SessionView): Session {
  return {
    id: view.name,
    role: view.role,
    identity: view.identity,
    seat: view.seat,
    state: view.ready ? "idle" : "working",
    pending: view.pending,
    drafting: view.kai_drafting,
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

  constructor(url = DEFAULT_DAEMON_URL) {
    this.socket = new WebSocket(url);
    this.socket.addEventListener("open", () => this.socket.send(JSON.stringify({ type: "hello", format: FORMAT })));
    this.socket.addEventListener("message", (event) => this.receive(JSON.parse(String(event.data)) as Frame));
    this.socket.addEventListener("close", () => this.emit({ type: "closed", reason: "The daemon closed the connection." }));
  }

  subscribe(listener: (event: HostEvent) => void): () => void {
    this.listeners.add(listener);
    if (this.last.roles) listener(this.last.roles);
    if (this.last.sessions) listener({ type: "sessions", sessions: this.last.sessions });
    return () => this.listeners.delete(listener);
  }

  attach(sessionId: string, rows: number, cols: number): void {
    this.request({ type: "attach", session: sessionId, replay: true, rows, cols });
  }

  detach(sessionId: string): void {
    this.request({ type: "detach", session: sessionId });
  }

  input(sessionId: string, data: string): void {
    this.request({ type: "input", session: sessionId, data: encode(data) });
  }

  resize(sessionId: string, rows: number, cols: number): void {
    this.request({ type: "resize", session: sessionId, rows, cols });
  }

  launch(role: string, seat: string): void {
    const id = this.request({ type: "launch", role, seat });
    this.launches.set(id, role);
    this.emit({ type: "launch", role, state: "starting", text: `Opening ${role} on ${seat}. Its window opens on the host.` });
  }

  close(): void {
    this.listeners.clear();
    this.socket.close();
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
      case "sessions":
        this.last.sessions = (frame.sessions ?? []).map(toSession);
        this.emit({ type: "sessions", sessions: this.last.sessions });
        return;
      case "roster":
        this.last.roles = { type: "roster", roles: parseRoster(frame.roster) };
        this.emit(this.last.roles);
        return;
      case "output":
        if (frame.session && frame.data) this.emit({ type: "output", sessionId: frame.session, data: decode(frame.data) });
        return;
      case "message":
        if (frame.message) this.emit({ type: "message", message: toMessage(frame.message) });
        return;
      case "launched":
        if (frame.id) this.launches.delete(frame.id);
        this.emit({ type: "launch", role: frame.role ?? "", state: "started", text: `${frame.role} launched. It appears here once its session starts.` });
        return;
      case "error": {
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
