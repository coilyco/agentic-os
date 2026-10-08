// aterm.daemon.v1 over the daemon's loopback websocket. The frame reference is
// the Wire contract section of the aterm daemon page in coilyco/agentic-os.
import { parseFrom } from "./messages";
import type { ToolResult, View, ViewCsp } from "./mcp-apps";
import type { Ask, AskOutcome, ContextReading, HostConnection, HostEvent, MessageState, PeerMessage, Session, TerminalChannel, ViewChannel } from "./protocol";
import { parseRoster } from "./roster";
import { parseTyping, refusalOf, typingNotice, type Typing } from "./typing";

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
  degraded?: string[];
  context?: { tokens: number; window?: number; source: string };
}

// A plain shell as the daemon lists it (COI-2498). It never names the seat beside it.
interface TerminalView {
  name: string;
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
  terminals?: TerminalView[];
  ask_id?: string;
  state?: string;
  typing?: unknown;
  reason?: unknown;
  features?: string[];
  view?: ViewFrame;
  view_id?: string;
  tool_result?: ToolResult;
  cancelled?: string;
  result?: unknown;
}

interface ViewFrame {
  id: string;
  server: string;
  tool: string;
  resource_uri: string;
  html: string;
  csp?: { connect_domains?: string[]; resource_domains?: string[]; frame_domains?: string[]; base_uri_domains?: string[] };
  prefers_border?: boolean;
  tool_input?: Record<string, unknown>;
}

/** A `view` frame as the client holds it. The result comes later, in `view_update`. */
export function toView(session: string, view: ViewFrame): View {
  const csp: ViewCsp = {};
  if (view.csp?.connect_domains) csp.connectDomains = view.csp.connect_domains;
  if (view.csp?.resource_domains) csp.resourceDomains = view.csp.resource_domains;
  if (view.csp?.frame_domains) csp.frameDomains = view.csp.frame_domains;
  if (view.csp?.base_uri_domains) csp.baseUriDomains = view.csp.base_uri_domains;
  return {
    id: view.id,
    session,
    server: view.server,
    tool: view.tool,
    resourceUri: view.resource_uri,
    html: view.html,
    toolInput: view.tool_input ?? {},
    ...(view.csp ? { csp } : {}),
    ...(view.prefers_border === undefined ? {} : { prefersBorder: view.prefers_border }),
  };
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
    degraded: view.degraded ?? [],
    ...(view.ready ? {} : { starting: true }),
    ...(view.context ? { context: toContext(view.context) } : {}),
  };
}

function toContext(view: NonNullable<SessionView["context"]>): ContextReading {
  return { tokens: view.tokens, source: view.source, ...(view.window ? { window: view.window } : {}) };
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

/** What a person reads when the daemon refuses to open a terminal. */
export function terminalRefusal(reason: unknown, error: string | undefined): string {
  if (reason === "remote_terminal") return "This device cannot open a terminal on the host yet. Open one from the host's own browser.";
  return error ?? "The host would not open a terminal.";
}

export class DaemonHost implements HostConnection {
  readonly canLaunch = true;
  readonly terminals: TerminalChannel = {
    open: (label) => void this.pendingOpens.set(this.request({ type: "spawn", kind: "terminal" }), label),
    close: (id) => void this.request({ type: "close", target: id }),
  };
  // Spawn request ids to the seat they were asked for, until the daemon answers.
  private pendingOpens = new Map<string, string>();
  // Every terminal name seen. Not a seat: never watched for busy, its exit is its own.
  private terminalNames = new Set<string>();
  private launches = new Map<string, string>();
  private features = new Set<string>();
  private calls = new Map<string, { resolve: (result: unknown) => void; reject: (error: Error) => void }>();
  private socket: WebSocket;
  private listeners = new Set<(event: HostEvent) => void>();
  private outbox: string[] = [];
  private open = false;
  private nextId = 0;
  private last: { sessions?: Session[]; roles?: HostEvent; typing?: Typing; features?: HostEvent; terminals?: HostEvent } = {};
  private sessionViews: SessionView[] = [];
  private refs = new Map<string, number>();
  private lastOutput = new Map<string, number>();
  private lastPoke = new Map<string, number>();
  private busy = new Set<string>();
  private ticker: ReturnType<typeof setInterval>;

  constructor(url = DEFAULT_DAEMON_URL) {
    this.socket = new WebSocket(url);
    this.socket.addEventListener("open", () => this.socket.send(JSON.stringify({ type: "hello", format: FORMAT })));
    this.socket.addEventListener("message", (event) => this.receive(JSON.parse(String(event.data)) as Frame));
    this.socket.addEventListener("close", () => {
      for (const call of this.calls.values()) call.reject(new Error("The daemon closed the connection."));
      this.calls.clear();
      this.emit({ type: "closed", reason: "The daemon closed the connection." });
    });
    this.ticker = setInterval(() => this.settle(), 500);
  }

  subscribe(listener: (event: HostEvent) => void): () => void {
    this.listeners.add(listener);
    if (this.last.roles) listener(this.last.roles);
    if (this.last.sessions) listener({ type: "sessions", sessions: this.last.sessions });
    if (this.last.typing) listener({ type: "typing", typing: this.last.typing });
    if (this.last.features) listener(this.last.features);
    if (this.last.terminals) listener(this.last.terminals);
    return () => this.listeners.delete(listener);
  }

  /** Only a welcome listing `mcp-apps` forwards views. Otherwise the panel says so. */
  get views(): ViewChannel | undefined {
    return this.features.has("mcp-apps") ? this.viewChannel : undefined;
  }

  private viewChannel: ViewChannel = {
    call: (viewId, method, params) =>
      new Promise((resolve, reject) => {
        this.calls.set(this.request({ type: "view_call", view_id: viewId, method, params }), { resolve, reject });
      }),
    close: (viewId) => {
      this.request({ type: "view_close", view_id: viewId });
    },
  };

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
    const live = new Set(this.sessionViews.map((view) => view.name));
    for (const name of live) {
      if (this.refs.has(name)) continue;
      this.refs.set(name, 1);
      this.request({ type: "attach", session: name, replay: false });
    }
    for (const name of [...this.refs.keys()]) {
      if (!live.has(name) && !this.terminalNames.has(name)) {
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
    if (this.terminalNames.has(name) || now - (this.lastPoke.get(name) ?? 0) < SELF_WINDOW_MS) return;
    this.lastOutput.set(name, now);
    if (!this.busy.has(name)) {
      this.busy.add(name);
      this.publishSessions();
    }
  }

  private publishSessions(): void {
    this.last.sessions = this.sessionViews.map((view) => toSession(view, this.busy.has(view.name)));
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
        this.setTyping(parseTyping(frame.typing));
        this.features = new Set(frame.features ?? []);
        this.last.features = { type: "features", terminals: this.features.has("terminals") };
        this.emit(this.last.features);
        this.request({ type: "subscribe", channel: "sessions" });
        this.request({ type: "roster" });
        if (this.features.has("mcp-apps")) this.request({ type: "subscribe", channel: "views" });
        for (const line of this.outbox.splice(0)) this.socket.send(line);
        return;
      case "ask":
        if (frame.ask) this.emit({ type: "ask", ask: toAsk(frame.ask) });
        return;
      case "asked":
        if (frame.ask_id) this.emit({ type: "asked", id: frame.ask_id, outcome: (frame.state ?? "answered") as AskOutcome });
        return;
      case "sessions":
        this.sessionViews = frame.sessions ?? [];
        for (const { name } of frame.terminals ?? []) this.terminalNames.add(name);
        this.monitor();
        this.publishSessions();
        // Absent means none, so a daemon with no shells still reports a list.
        this.last.terminals = { type: "terminals", terminals: (frame.terminals ?? []).map(({ name }) => ({ id: name })) };
        this.emit(this.last.terminals);
        return;
      case "spawned": {
        const label = frame.id ? this.pendingOpens.get(frame.id) : undefined;
        if (label === undefined || !frame.session) return;
        this.pendingOpens.delete(frame.id!);
        this.terminalNames.add(frame.session);
        this.emit({ type: "terminal_opened", label, id: frame.session });
        return;
      }
      case "exit":
      case "exited":
        if (frame.session && this.terminalNames.has(frame.session)) this.emit({ type: "terminal_exit", id: frame.session, code: frame.code ?? 0 });
        return;
      case "view":
        if (frame.view && frame.session) this.emit({ type: "view", view: toView(frame.session, frame.view) });
        return;
      case "view_update":
        if (frame.view_id) this.emit({ type: "view_update", id: frame.view_id, ...(frame.tool_result ? { toolResult: frame.tool_result } : {}), ...(frame.cancelled ? { cancelled: frame.cancelled } : {}) });
        return;
      case "view_closed":
        if (frame.view_id) this.emit({ type: "view_closed", id: frame.view_id });
        return;
      case "view_result": {
        const call = frame.id ? this.calls.get(frame.id) : undefined;
        if (call && frame.id) {
          this.calls.delete(frame.id);
          call.resolve(frame.result);
        }
        return;
      }
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
        const refused = refusalOf(frame);
        this.setTyping(refused);
        const call = frame.id ? this.calls.get(frame.id) : undefined;
        if (call && frame.id) {
          this.calls.delete(frame.id);
          call.reject(new Error(refused ? typingNotice(refused) : (frame.error ?? "The daemon refused the view call.")));
          return;
        }
        const seat = frame.id ? this.pendingOpens.get(frame.id) : undefined;
        if (seat !== undefined && frame.id) {
          this.pendingOpens.delete(frame.id);
          this.emit({ type: "terminal_refused", label: seat, text: refused ? typingNotice(refused) : terminalRefusal(frame.reason, frame.error) });
          return;
        }
        if (role !== undefined && frame.id) {
          this.launches.delete(frame.id);
          this.emit({ type: "launch", role, state: "failed", text: refused ? typingNotice(refused) : launchRefusal(frame.code, frame.error) });
        } else if (!refused) {
          this.emit({ type: "notice", text: frame.error ?? "The daemon refused a request." });
        }
        return;
      }
    }
  }

  private setTyping(typing: Typing | null): void {
    if (!typing) return;
    this.last.typing = typing;
    this.emit({ type: "typing", typing });
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
