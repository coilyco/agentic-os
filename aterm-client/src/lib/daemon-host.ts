// aterm.daemon.v1 over the daemon's loopback websocket. The frame reference is
// the Wire contract section of the aterm daemon page in coilyco/agentic-os.
import { quietSince } from "./age";
import { ScreenTails } from "./screen-tail";
import { toActivity } from "./seat-activity";
import { backoffDelay, differentBuild } from "./backoff";
import { safely } from "./pane-watch";
import { parseFrom } from "./messages";
import type { ToolResult, View, ViewCsp } from "./mcp-apps";
import { FeedbackRefused, feedbackRefusal } from "./feedback";
import { IMAGE_UPLOAD_MAX_BYTES, imageRefusal } from "./images";
import { CHUNK_BYTES, decodeMedia, mediaRefusal } from "./media";
import type { Ask, AskOutcome, BrowserChannel, ContextReading, DeviceKeyChannel, FeedbackChannel, HostConnection, HostEvent, ImageChannel, LastReplyChannel, MediaChannel, MessageState, PasskeyChannel, PeerMessage, PushChannel, Session, TerminalChannel, TranscriptChannel, ViewChannel } from "./protocol";
import { decodeReply, REFUSALS, TranscriptRefused } from "./transcript";
import { parseRoster } from "./roster";
import type { BrowserState, Driver, FrameMetadata, InputKind, SharedBrowser } from "./screencast";
import { inAndroidApp } from "./open-external";
import { bridge, DeviceKeyError, type Invoke, enrollKey, keyStatus, KEY_CODE_SPENT, mayEnroll, signChallenge, biometricNotice } from "./device-key";
import { CODE_SPENT, createCredential, getCredential, passkeySupport, PasskeyError } from "./passkey";
import { RESUME_ENDED_REASON, RESUME_FEATURE, ResumeEndedError, resumeProof } from "./resume";
import { keepStanding, parseTyping, Refusal, refusalOf, typingNotice, type Typing } from "./typing";

export const FORMAT = "aterm.daemon.v1";

/** How a brokered link gets its channel and unlock (references/treehouse-broker.md) */
export interface Brokered {
  /** A fresh one-use channel URL. Asked for on every dial, since a ticket opens once. */
  dial(): Promise<string>;
  /** A single-use unlock token for this daemon and nonce, asked for just before use. */
  unlock(daemonId: string, nonce: string): Promise<string>;
}

/** `welcome.features` names this on a channel the broker opened, which starts locked. */
const UNLOCK_FEATURE = "unlock-token";
/** The daemon spends a nonce on the fifth refusal, so move to a new channel first. */
const UNLOCK_RETRIES = 3;
const SESSION_OVER = "Your sign-in ended. Sign in again to reconnect.";
const MACHINE_GONE = "This machine is no longer on your account.";
const MACHINE_OFFLINE = "This machine is offline. It is not connected to the broker.";

/** Why a ticket could not be had, and whether waiting would help. One shape for both. */
function dialFailure(error: unknown, answered: boolean): { reason: string; final: boolean } {
  const code = (error as { code?: unknown } | null)?.code;
  if (code === "unauthorized") return { reason: SESSION_OVER, final: true };
  if (code === "not_found") return { reason: MACHINE_GONE, final: true };
  const reason = code === "daemon_offline" ? MACHINE_OFFLINE : error instanceof Error ? error.message : "The connection failed.";
  return { reason, final: !answered };
}
const LOOPBACK_DAEMON = "ws://127.0.0.1:7419";
// A sleeping tailnet path can hold a dial open for minutes. Give up on it sooner.
const DIAL_TIMEOUT_MS = 10000;
// A link that carries nothing back is dead, whatever the socket says: a slept phone
// leaves a socket open that never closes (COI-3352). Any frame counts.
const PROBE_REPLY_MS = 8000;
// A return to the page replaces a dial that has been out this long, and spares a dial
// that only just left, so a burst of focus events does not redial in a loop.
const WAKE_REDIAL_AFTER_MS = 1500;
/** Input ids kept so an error naming one can be told from any other. */
const INPUT_IDS_KEPT = 64;
const NOT_SENT_CLOSED = "Not sent. The connection to the host closed, and it is reconnecting.";
const PTY_SIZE_FEATURE = "pty-size";
const NO_PLUGIN = "This app has no way to hold a key on this phone.";
const NO_KEY_YET = "This phone has no key yet. Set one up with a code first.";
const UNLOCK_PROMPT = "Unlock Treehouse to type";
const KEY_SET_UP = "Your key is set up, so the Unlock button will work.";
const KEY_UNKNOWN_REASON = "device_key_unknown";
const KEY_UNKNOWN = "This host does not know this phone's key. Set one up again with a new code.";

/** A queued attach with a box says it pans, once the welcome lists pty-size. */
function withScales(line: string): string {
  const frame = JSON.parse(line) as { type?: string; rows?: number };
  return frame.type === "attach" && frame.rows !== undefined ? JSON.stringify({ ...frame, scales: true }) : line;
}

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
  /** When this run of the seat began, which tells a restarted seat from the same one. */
  started?: string;
  pid?: number;
  role: string;
  identity: string;
  seat: string;
  ready: boolean;
  bracketed_paste: boolean;
  kai_drafting: boolean;
  pending: number;
  degraded?: string[];
  context?: { tokens: number; window?: number; source: string };
  last_reply?: string;
  /** When that reply ended (RFC 3339). */
  last_reply_at?: string;
  /** Seconds since the seat last wrote, as of the frame. */
  quiet_seconds?: number;
  state_seconds?: number;
  state?: string;
  ref?: string;
  origin?: string;
  background?: number;
  /** The seat's own line about what it is doing, and when (RFC 3339). */
  activity?: string;
  activity_at?: string;
}

// A plain shell as the daemon lists it (COI-2498). `label` is whatever the opener sent
// on the spawn (COI-2535), absent from a daemon without the `terminal-label` feature.
interface TerminalView {
  name: string;
  label?: string;
}

interface MessageView {
  id: string;
  from: string;
  target: string;
  session?: string;
  state: MessageState;
  reason?: string;
  text?: string;
  text_cut?: boolean;
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
  relay?: boolean;
  card_seat?: string;
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
    ...(view.relay ? { relay: true } : {}),
    ...(view.card_seat ? { cardSeat: view.card_seat } : {}),
  };
}

interface Frame {
  type: string;
  id?: string;
  format?: string;
  /** The daemon's build, on its welcome. */
  version?: string;
  session?: string;
  data?: string;
  /** The absolute file an `image_uploaded` names. */
  path?: string;
  /** The token `feedback_image_saved` names, and the record `feedback_saved` names. */
  image?: string;
  record?: string;
  /** A coordinator seat's ping, on the sessions channel. */
  notify?: { id?: string; session?: string; role?: string; identity?: string; title?: string; body?: string; at?: string };
  /** Answers a `last_reply` request. */
  reply?: { session?: string; text?: string; truncated?: boolean; at?: string };
  /** Answers a `transcript` request. See the transcript reference. */
  items?: unknown;
  cursor?: string;
  more?: boolean;
  reader?: string;
  updated_at?: string;
  /** A shared file's descriptor on `media`, and the chunk's place on `media_data`. */
  media?: unknown;
  media_id?: string;
  offset?: number;
  size?: number;
  eof?: boolean;
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
  /** On a brokered welcome only: which daemon this is and the channel's nonce. */
  unlock?: { daemon_id?: string; nonce?: string };
  challenge?: string;
  rows?: number;
  cols?: number;
  label?: string;
  /** A staged command, and the seat's note on it. */
  body?: string;
  text?: string;
  driver?: string;
  url?: string;
  title?: string;
  holder?: string;
  tab?: number;
  tabs?: number;
  client?: string;
  seq?: number;
  metadata?: Record<string, number>;
  view?: ViewFrame;
  view_id?: string;
  artifact?: { id: string; title?: string; url?: string; html?: string; standing?: boolean; waiting?: boolean; seat_ended?: boolean };
  artifact_id?: string;
  waiting?: boolean;
  seat_ended?: boolean;
  tool_result?: ToolResult;
  cancelled?: string;
  result?: unknown;
  options?: unknown;
  resume?: string;
  expires_in?: number;
  /** The VAPID public key on a `push_key` reply. */
  key?: string;
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

/** `welcome.features` names this when the daemon streams a session's browser. */
export const BROWSER_FEATURE = "browser";

/** `welcome.features` names this when the daemon has a VAPID key and answers `push_*`. */
export const PUSH_FEATURE = "web-push";

/** Replies that unlock a connection. Each names the next resume proof (COI-3305). */
const UNLOCKED_REPLIES = new Set(["passkey_enrolled", "passkey_asserted", "device_asserted", "resumed"]);

/** The daemon keeps a `label` on a terminal spawn and lists it back. */
const TERMINAL_LABEL_FEATURE = "terminal-label";

/** `welcome.features` names this when the daemon sends a seat's `open_url` (COI-3258). */
const OPEN_URL_FEATURE = "open-url";

/** `welcome.features` names this when the daemon answers `feedback` (COI-3345). */
const FEEDBACK_FEATURE = "feedback";
/** The daemon holds a staged screenshot ten minutes. The client trusts it for nine. */
const FEEDBACK_TOKEN_MS = 9 * 60 * 1000;
/** `welcome.features` names this when the daemon answers `image_upload` (COI-3239). */
const IMAGE_UPLOAD_FEATURE = "image-upload";

/** CDP's ScreencastFrameMetadata as the daemon sends it, in snake_case. */
export function toFrameMetadata(raw: Record<string, number> | undefined): FrameMetadata {
  return {
    deviceWidth: raw?.device_width ?? 0,
    deviceHeight: raw?.device_height ?? 0,
    offsetTop: raw?.offset_top ?? 0,
    pageScaleFactor: raw?.page_scale_factor ?? 1,
  };
}

const BROWSER_STATES: BrowserState[] = ["none", "live", "closed"];

/** A `browser_state` over what the client already holds, which keeps the last frame. */
export function toBrowserState(frame: Frame, previous: SharedBrowser | undefined): SharedBrowser {
  const state = BROWSER_STATES.find((each) => each === frame.state) ?? "none";
  const driver: Driver = frame.driver === "person" ? "person" : "agent";
  return {
    session: frame.session ?? "",
    state,
    driver,
    url: frame.url ?? "",
    title: frame.title ?? "",
    // Only the daemon knows each connection's name, and it sends this one's as `client`.
    heldHere: driver === "person" && !!frame.holder && frame.holder === frame.client,
    ...(frame.holder ? { holder: frame.holder } : {}),
    ...(typeof frame.reason === "string" && frame.reason ? { reason: frame.reason } : {}),
    ...(frame.tab && frame.tabs ? { tab: frame.tab, tabs: frame.tabs } : {}),
    ...(previous?.frame ? { frame: previous.frame } : {}),
  };
}

/** A `browser_frame` drawn onto the browser it belongs to. Null before any state. */
export function withBrowserFrame(previous: SharedBrowser | undefined, frame: Frame, at: number): SharedBrowser | null {
  if (!previous || !frame.data) return null;
  return { ...previous, frame: { src: `data:image/jpeg;base64,${frame.data}`, metadata: toFrameMetadata(frame.metadata), at, seq: frame.seq ?? 0 } };
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
/** How often the seats are listed again while the page shows, to refresh quiet times. */
const REFRESH_MS = 20_000;

const SCREEN_STATES = ["starting", "busy", "idle", "prompt"];

export function toSession(view: SessionView, busy = false, quiet?: number, receivedAt?: number): Session {
  const activity = toActivity(view.activity, view.activity_at);
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
    ...(SCREEN_STATES.includes(view.state as never) ? { screen: view.state as NonNullable<Session["screen"]> } : {}),
    ...(typeof view.background === "number" ? { background: view.background } : {}),
    ...(view.ref ? { ref: view.ref } : {}),
    ...(view.origin === "window" || view.origin === "headless" ? { origin: view.origin } : {}),
    ...(view.context ? { context: toContext(view.context) } : {}),
    ...(view.last_reply ? { lastReply: view.last_reply } : {}),
    ...(view.last_reply && Number.isFinite(Date.parse(view.last_reply_at ?? "")) ? { lastReplyAt: Date.parse(view.last_reply_at ?? "") } : {}),
    ...(activity ? { activity } : {}),
    ...(quiet === undefined ? {} : { quietSince: quiet }),
    ...(receivedAt === undefined || typeof view.state_seconds !== "number" ? {} : { stateSince: receivedAt - view.state_seconds * 1000 }),
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
    text: view.text ?? null,
    textCut: view.text_cut === true,
  };
}

function encode(text: string): string {
  const bytes = new TextEncoder().encode(text);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function encodeBytes(bytes: Uint8Array): string {
  // Chunked, since spreading megabytes into one call overflows the stack.
  let binary = "";
  for (let at = 0; at < bytes.length; at += 0x8000) binary += String.fromCharCode(...bytes.subarray(at, at + 0x8000));
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
    open: (label) =>
      void this.pendingOpens.set(this.request({ type: "spawn", kind: "terminal", ...(this.features.has(TERMINAL_LABEL_FEATURE) ? { label } : {}) }), label),
    close: (id) => void this.request({ type: "close", target: id }),
  };
  // Spawn request ids to the seat they were asked for, until the daemon answers.
  private pendingOpens = new Map<string, string>();
  // Every terminal name seen. Not a seat: never watched for busy, its exit is its own.
  private terminalNames = new Set<string>();
  private launches = new Map<string, string>();
  // Close request ids to the seat they name, until the daemon answers.
  private closes = new Map<string, string>();
  private reopens = new Map<string, string>();
  private features = new Set<string>();
  private calls = new Map<string, { resolve: (reply: Frame) => void; reject: (error: Error) => void }>();
  private socket?: WebSocket;
  private listeners = new Set<(event: HostEvent) => void>();
  private outbox: string[] = [];
  private open = false;
  // A welcome has come at least once, so a lost socket is a restart to ride out. Before
  // that the host never answered, and the person is told to start it.
  private answered = false;
  // Set by close() and by a refused welcome: no more dials.
  private halted = false;
  // The hour of automatic tries is spent. Only a return to the page or Retry dials again.
  private resting = false;
  // The socket that already ended, so a late close from it is not a second loss.
  private ended?: WebSocket;
  private dialStartedAt = 0;
  // When a frame last arrived, and when the probe still waiting on one went out.
  private lastHeard = 0;
  private probeAt?: number;
  // The ticket request in flight: an answer after a newer dial or a close is dropped.
  private turn = 0;
  // What a brokered welcome gave this channel, and how often a token was refused on it.
  private channel?: { daemonId: string; nonce: string; refusals: number };
  private attempt = 0;
  private redial?: ReturnType<typeof setTimeout>;
  private dialTimer?: ReturnType<typeof setTimeout>;
  // The build that answered first, which is the one that served this page.
  private servedBy?: string;
  // Set on a welcome after a loss, until the sessions list says what to attach.
  private resync = false;
  // Terminal attaches by seat, so a new connection can attach them again with replay.
  private attached = new Map<string, { rows: number; cols: number; count: number }>();
  private watching = new Map<string, { width: number; height: number } | undefined>();
  private nextId = 0;
  // Input has no reply on success, so an error naming one of these is its only failure.
  private inputIds = new Set<string>();
  private last: { sessions?: Session[]; roles?: HostEvent; typing?: Typing; features?: HostEvent; passkey?: HostEvent; deviceKey?: HostEvent; terminals?: HostEvent } = {};
  private sessionViews: SessionView[] = [];
  // The daemon's one-use unlock proof. Memory only, replaced on every use.
  private resumeToken: string | undefined;
  private refs = new Map<string, number>();
  private lastOutput = new Map<string, number>();
  private lastPoke = new Map<string, number>();
  // What arrived since a seat's last attach, so an ask-again can place a lost replay.
  private arrived = new Map<string, { since: number; frames: number; bytes: number }>();
  // What each opened seat's terminal drew, so a reopen asks only for the rest.
  private tails = new ScreenTails();
  private busy = new Set<string>();
  // When the latest `sessions` frame arrived. Its `quiet_seconds` count back from here.
  private viewsAt = 0;
  private ticker: ReturnType<typeof setInterval>;
  private refresher: ReturnType<typeof setInterval>;
  private browsers = new Map<string, SharedBrowser>();
  /** Set once the welcome lists `browser`, so an older daemon keeps the empty state. */
  browser?: BrowserChannel;

  constructor(
    private url = DEFAULT_DAEMON_URL,
    private via?: Brokered,
  ) {
    this.connect();
    this.ticker = setInterval(() => this.settle(), 500);
    // The daemon pushes sessions on change, not on a clock, so quiet times are asked for.
    this.refresher = setInterval(() => this.refreshViews(), REFRESH_MS);
    // A backgrounded tab freezes its timers, so a phone left open redials late or never.
    // Coming back to it is the signal to look at the link at once.
    if (typeof window !== "undefined") {
      window.addEventListener("online", this.wake);
      window.addEventListener("focus", this.wake);
      document.addEventListener("visibilitychange", this.wake);
    }
  }

  /** True on a connection the broker routes, which the daemon starts locked. */
  get brokered(): boolean {
    return this.via !== undefined;
  }

  private connect(): void {
    const via = this.via;
    if (!via) return this.dial(this.url);
    const turn = ++this.turn;
    via.dial().then(
      (url) => {
        if (turn === this.turn && !this.halted) this.dial(url);
      },
      (error: unknown) => {
        if (turn === this.turn && !this.halted) this.dialFailed(error);
      },
    );
  }

  private dialFailed(error: unknown): void {
    const { reason, final } = dialFailure(error, this.answered);
    // No socket ended here, so the guard that drops a second close of one does not apply.
    if (!final) return this.lost(true);
    this.halted = true;
    this.emit({ type: "closed", reason });
  }

  private dial(url: string): void {
    const socket = new WebSocket(url);
    this.socket = socket;
    this.dialStartedAt = Date.now();
    // A socket this host has moved past must not speak for it.
    socket.addEventListener("open", () => socket.send(JSON.stringify({ type: "hello", format: FORMAT })));
    socket.addEventListener("message", (event) => {
      if (socket !== this.socket) return;
      this.lastHeard = Date.now();
      this.probeAt = undefined;
      this.receive(JSON.parse(String(event.data)) as Frame);
    });
    socket.addEventListener("close", () => {
      if (socket === this.socket) this.lost();
    });
    this.dialTimer = setTimeout(() => socket.close(), DIAL_TIMEOUT_MS);
  }

  // The socket ended. A host that answered is redialed and the page keeps its state.
  // One that never answered is reported closed.
  private lost(ticketFailed = false): void {
    if (!ticketFailed && this.ended === this.socket) return;
    this.ended = this.socket;
    clearTimeout(this.dialTimer);
    this.open = false;
    this.probeAt = undefined;
    // A new channel has a new nonce, so nothing of this one can unlock it.
    this.channel = undefined;
    for (const call of this.calls.values()) call.reject(new Error("The daemon closed the connection."));
    this.calls.clear();
    if (this.halted) return;
    if (!this.answered) {
      this.emit({ type: "closed", reason: "The daemon closed the connection." });
      return;
    }
    // Replies will not come, and keystrokes sent into a dead link must not land later.
    this.pendingOpens.clear();
    this.closes.clear();
    this.reopens.clear();
    this.launches.clear();
    this.outbox = [];
    const delay = backoffDelay(this.attempt);
    if (delay === null) {
      this.resting = true;
      this.emit({ type: "resting" });
      return;
    }
    this.attempt += 1;
    this.emit({ type: "reconnecting", attempt: this.attempt, retryAt: Date.now() + delay });
    this.redial = setTimeout(() => this.dialAgain(), delay);
  }

  private dialAgain(): void {
    this.redial = undefined;
    this.emit({ type: "reconnecting", attempt: this.attempt, retryAt: null });
    this.connect();
  }

  /** Dials now, from the Retry button. A dial already out is left to finish. A person
   * asking starts the hour of tries over, since the first wait is the short one. */
  retry(): void {
    if (this.redial === undefined && !this.resting) return;
    clearTimeout(this.redial);
    this.resting = false;
    this.attempt = 0;
    this.dialAgain();
  }

  // The person came back, or the network did. Whatever the link claims, look at it now.
  private wake = (): void => {
    if (this.halted || (typeof document !== "undefined" && document.visibilityState !== "visible")) return;
    if (this.redial !== undefined || this.resting) this.retry();
    else if (this.open) this.probe();
    else if (Date.now() - this.dialStartedAt >= WAKE_REDIAL_AFTER_MS) {
      // A dial still out may sit on a path that is gone. Drop it for one on the path now.
      this.ended = this.socket;
      clearTimeout(this.dialTimer);
      this.socket?.close();
      this.connect();
    }
  };

  // Asks for something small, so a link that carries nothing back shows within seconds.
  // The reply, or any other frame, clears it in the message handler.
  private probe(): void {
    if (!this.open) return;
    this.probeAt ??= Date.now();
    this.request({ type: "list" });
  }

  private watchLink(now: number): void {
    if (!this.open || this.probeAt === undefined || now - this.probeAt < PROBE_REPLY_MS) return;
    // A half-open socket may never fire close, so the loss is declared here.
    const socket = this.socket;
    this.lost();
    socket?.close();
  }

  subscribe(listener: (event: HostEvent) => void): () => void {
    this.listeners.add(listener);
    const tell = (event: HostEvent) => safely("a host listener", () => listener(event));
    if (this.last.roles) tell(this.last.roles);
    if (this.last.sessions) tell({ type: "sessions", sessions: this.last.sessions });
    if (this.last.typing) tell({ type: "typing", typing: this.last.typing });
    if (this.last.passkey) tell(this.last.passkey);
    if (this.last.deviceKey) tell(this.last.deviceKey);
    if (this.last.features) tell(this.last.features);
    if (this.last.terminals) tell(this.last.terminals);
    return () => this.listeners.delete(listener);
  }

  /** Only a welcome listing `mcp-apps` forwards views. Otherwise the panel says so. */
  get views(): ViewChannel | undefined {
    return this.features.has("mcp-apps") ? this.viewChannel : undefined;
  }

  private viewChannel: ViewChannel = {
    call: async (viewId, method, params) => (await this.exchange({ type: "view_call", view_id: viewId, method, params })).result,
    close: (viewId) => {
      this.request({ type: "view_close", view_id: viewId });
    },
  };

  get ptySize(): boolean {
    return this.features.has(PTY_SIZE_FEATURE);
  }

  /** Only a welcome listing `image-upload` takes an image for a seat. */
  get images(): ImageChannel | undefined {
    return this.features.has(IMAGE_UPLOAD_FEATURE) ? this.imageChannel : undefined;
  }

  private imageChannel: ImageChannel = {
    maxBytes: IMAGE_UPLOAD_MAX_BYTES,
    upload: async (sessionId, image) => {
      const data = encodeBytes(new Uint8Array(await image.arrayBuffer()));
      const reply = await this.exchange({ type: "image_upload", session: sessionId, data });
      if (!reply.path) throw new Error("The host took the image but named no file for it.");
      return reply.path;
    },
  };

  /** Only a welcome listing `feedback` keeps a note and screenshots as a record. */
  get feedback(): FeedbackChannel | undefined {
    return this.features.has(FEEDBACK_FEATURE) ? this.feedbackChannel : undefined;
  }

  /** Requests for feedback, whose refusal reasons are this feature's own. */
  private feedbackIds = new Set<string>();
  // A refusal keeps the tokens on the host, so a retry sends only the closing frame.
  private staged = new WeakMap<Blob, { token: string; at: number }>();

  private async stage(image: Blob): Promise<string> {
    const held = this.staged.get(image);
    if (held && Date.now() - held.at < FEEDBACK_TOKEN_MS) return held.token;
    const data = encodeBytes(new Uint8Array(await image.arrayBuffer()));
    const reply = await this.exchange({ type: "feedback_image", data }, this.feedbackIds);
    if (!reply.image) throw new Error("The host took the screenshot but gave no token for it.");
    this.staged.set(image, { token: reply.image, at: Date.now() });
    return reply.image;
  }

  private feedbackChannel: FeedbackChannel = {
    maxBytes: IMAGE_UPLOAD_MAX_BYTES,
    send: async (report, progress) => {
      const total = report.images.length;
      const attempt = async (): Promise<string> => {
        const tokens: string[] = [];
        for (const [done, image] of report.images.entries()) {
          progress?.(done, total);
          tokens.push(await this.stage(image));
        }
        progress?.(total, total);
        const reply = await this.exchange({ type: "feedback", text: report.text, screen: report.context, images: tokens }, this.feedbackIds);
        if (!reply.record) throw new Error("The host took the feedback but named no record for it.");
        return reply.record;
      };
      try {
        return await attempt();
      } catch (error) {
        if (!(error instanceof FeedbackRefused) || error.reason !== "image_expired") throw error;
        // The tokens died, from age or a redial. The pictures go up once more.
        for (const image of report.images) this.staged.delete(image);
        return attempt();
      }
    },
  };

  /** Only a welcome listing `last-reply` answers for a seat's whole last reply. */
  get lastReplies(): LastReplyChannel | undefined {
    return this.features.has("last-reply") ? this.lastReplyChannel : undefined;
  }

  /** Only a welcome listing `transcript` answers for a seat's transcript. */
  get transcripts(): TranscriptChannel | undefined {
    return this.features.has("transcript") ? this.transcriptChannel : undefined;
  }

  private transcriptChannel: TranscriptChannel = {
    read: async (sessionId, options = {}) => {
      const frame = await this.exchange({ type: "transcript", session: sessionId, ...options }, this.transcriptIds);
      return decodeReply(sessionId, frame as unknown as Record<string, unknown>);
    },
  };

  /** Only a welcome listing `media` sends the files a seat shares. */
  get media(): MediaChannel | undefined {
    return this.features.has("media") ? this.mediaChannel : undefined;
  }

  private mediaChannel: MediaChannel = {
    read: async (item, onProgress) => {
      const parts: Uint8Array[] = [];
      let offset = 0;
      for (;;) {
        const frame = await this.exchange({ type: "media_read", media_id: item.id, offset, length: CHUNK_BYTES });
        const bytes = frame.data ? decode(frame.data) : new Uint8Array();
        parts.push(bytes);
        offset += bytes.length;
        onProgress?.(offset);
        if (frame.eof === true || bytes.length === 0) break;
      }
      return new Blob(parts as BlobPart[], { type: item.mime });
    },
  };

  private lastReplyChannel: LastReplyChannel = {
    fetch: async (sessionId) => {
      const frame = await this.exchange({ type: "last_reply", target: sessionId });
      return { text: frame.reply?.text ?? "", truncated: frame.reply?.truncated === true };
    },
  };

  /** Only a welcome listing `passkey` has a ceremony. The page decides if it can run. */
  get passkey(): PasskeyChannel | undefined {
    return this.features.has("passkey") ? this.passkeyChannel : undefined;
  }

  /** Only a welcome listing `device-key` has one. Without the plugin it says so. */
  get deviceKey(): DeviceKeyChannel | undefined {
    return this.features.has("device-key") ? this.deviceKeyChannel : undefined;
  }

  // Enrolling does not unlock the connection, so an assertion follows it at once.
  private assertDeviceKey = async (invoke: Invoke): Promise<void> => {
    const status = await keyStatus(invoke);
    if (!status.enrolled || !status.keyId) throw new DeviceKeyError(NO_KEY_YET);
    const notice = biometricNotice(status.biometric);
    if (notice) throw new DeviceKeyError(notice);
    const begun = await this.exchange({ type: "device_assert_begin", key_id: status.keyId });
    const signed = await signChallenge(invoke, String(begun.challenge), UNLOCK_PROMPT);
    await this.exchange({ type: "device_assert_finish", key_id: signed.keyId, signature: signed.signature });
  };

  private deviceKeyChannel: DeviceKeyChannel = {
    status: async () => {
      const invoke = bridge();
      if (!invoke) throw new DeviceKeyError(NO_PLUGIN);
      return keyStatus(invoke);
    },
    enroll: async (code) => {
      const invoke = bridge();
      if (!invoke) throw new DeviceKeyError(NO_PLUGIN);
      // The code is spent at begin, so a phone that cannot ask is caught first.
      const status = await keyStatus(invoke);
      if (!mayEnroll(status)) throw new DeviceKeyError(biometricNotice(status.biometric) ?? NO_PLUGIN);
      const begun = await this.exchange({ type: "device_enroll_begin", enroll_code: code });
      try {
        const key = await enrollKey(invoke, String(begun.challenge));
        await this.exchange({ type: "device_enroll_finish", public_key: key.publicKey, attestation: key.attestation });
      } catch (error) {
        throw new DeviceKeyError(`${error instanceof Error ? error.message : String(error)} ${KEY_CODE_SPENT}`);
      }
      try {
        await this.assertDeviceKey(invoke);
      } catch (error) {
        const reason = error instanceof DeviceKeyError ? error.reason : undefined;
        throw new DeviceKeyError(`${error instanceof Error ? error.message : String(error)} ${KEY_SET_UP}`, reason);
      }
    },
    assert: async () => {
      const invoke = bridge();
      if (!invoke) throw new DeviceKeyError(NO_PLUGIN);
      await this.assertDeviceKey(invoke);
    },
  };

  private passkeyChannel: PasskeyChannel = {
    enroll: async (code) => {
      const unsupported = passkeySupport();
      if (unsupported) throw new PasskeyError(unsupported);
      const begun = await this.exchange({ type: "passkey_enroll_begin", enroll_code: code });
      try {
        const credential = await createCredential(begun.options);
        await this.exchange({ type: "passkey_enroll_finish", credential });
      } catch (error) {
        throw new PasskeyError(`${error instanceof Error ? error.message : String(error)} ${CODE_SPENT}`);
      }
    },
    assert: async () => {
      const unsupported = passkeySupport();
      if (unsupported) throw new PasskeyError(unsupported);
      const begun = await this.exchange({ type: "passkey_assert_begin" });
      const credential = await getCredential(begun.options);
      await this.exchange({ type: "passkey_assert_finish", credential });
    },
  };

  /** Only a welcome listing `web-push` answers `push_*`. */
  get push(): PushChannel | undefined {
    return this.features.has(PUSH_FEATURE) ? this.pushChannel : undefined;
  }

  private pushChannel: PushChannel = {
    key: async () => {
      const { key } = await this.exchange({ type: "push_key" });
      if (!key) throw new Error("The daemon sent no push key.");
      return key;
    },
    subscribe: async (push) => void (await this.exchange({ type: "push_subscribe", push })),
    unsubscribe: async (endpoint) => void (await this.exchange({ type: "push_unsubscribe", push: { endpoint } })),
  };

  /** One request, settled by the reply or the `error` that echoes its id. */
  private exchange(frame: Record<string, unknown>, track?: Set<string>): Promise<Frame> {
    return new Promise((resolve, reject) => {
      const id = this.request(frame);
      track?.add(id);
      this.calls.set(id, { resolve, reject });
    });
  }

  /** Requests for a transcript, whose refusal reasons are this feature's own. */
  private transcriptIds = new Set<string>();

  attach(sessionId: string, rows: number, cols: number): void {
    this.refs.set(sessionId, (this.refs.get(sessionId) ?? 0) + 1);
    this.attached.set(sessionId, { rows, cols, count: (this.attached.get(sessionId)?.count ?? 0) + 1 });
    this.lastPoke.set(sessionId, Date.now());
    this.arrived.set(sessionId, { since: Date.now(), frames: 0, bytes: 0 });
    this.request({ type: "attach", session: sessionId, replay: true, rows, cols, ...this.resumeFrom(sessionId), ...(this.ptySize ? { scales: true } : {}) });
  }

  // Draws what the terminal held at once and names where the host goes on from.
  private resumeFrom(sessionId: string): { offset?: number } {
    this.tails.keep(sessionId, this.runOf(sessionId));
    const held = this.tails.resume(sessionId);
    if (!held) return {};
    this.emit({ type: "output", sessionId, data: held.bytes });
    return { offset: held.end };
  }

  private runOf = (name: string): string => {
    const view = this.sessionViews.find((each) => each.name === name);
    return `${view?.started ?? ""}/${view?.pid ?? ""}`;
  };

  replay(sessionId: string): void {
    const shown = this.attached.get(sessionId);
    const got = this.arrived.get(sessionId);
    // Zero frames, socket open: lost before here. Frames that never drew: in the pane.
    console.warn(`aterm: asking ${sessionId} for its screen again, ${got ? `${got.frames} frame(s), ${got.bytes} byte(s) in ${Date.now() - got.since} ms since the last attach` : "no attach on record"}, socket ${this.open ? "open" : "closed"}, ${this.outbox.length} queued`);
    this.lastPoke.set(sessionId, Date.now());
    // The screen is asked for whole, since the one drawn never showed.
    this.tails.clear(sessionId);
    const box = shown ? { rows: shown.rows, cols: shown.cols, ...(this.ptySize ? { scales: true } : {}) } : {};
    this.request({ type: "attach", session: sessionId, replay: true, ...box });
  }

  // The activity monitor holds its own reference, so closing a terminal tab
  // leaves the seat watched and the daemon never sees a detach for it.
  detach(sessionId: string): void {
    const shown = this.attached.get(sessionId);
    if (shown && shown.count > 1) shown.count -= 1;
    else this.attached.delete(sessionId);
    const left = (this.refs.get(sessionId) ?? 1) - 1;
    if (left > 0) {
      this.refs.set(sessionId, left);
      return;
    }
    this.refs.delete(sessionId);
    this.arrived.delete(sessionId);
    this.request({ type: "detach", session: sessionId });
  }

  input(sessionId: string, data: string): void {
    this.lastPoke.set(sessionId, Date.now());
    // A closed socket drops what is sent. Say so rather than lose it unseen.
    if (!this.open && this.answered) {
      this.emit({ type: "input_refused", text: NOT_SENT_CLOSED });
      return;
    }
    this.inputIds.add(this.request({ type: "input", session: sessionId, data: encode(data) }));
    if (this.inputIds.size > INPUT_IDS_KEPT) this.inputIds.delete(this.inputIds.values().next().value as string);
  }

  resize(sessionId: string, rows: number, cols: number): void {
    const shown = this.attached.get(sessionId);
    if (shown) Object.assign(shown, { rows, cols });
    this.lastPoke.set(sessionId, Date.now());
    this.request({ type: "resize", session: sessionId, rows, cols });
  }

  answer(askId: string, picks: number[], text?: string): void {
    this.request({ type: "answer", ask_id: askId, picks, ...(text ? { text } : {}) });
  }

  cancelAsk(askId: string): void {
    this.request({ type: "cancel_ask", ask_id: askId });
  }

  closeSeat(session: string, ref: string, force = false): void {
    this.closes.set(this.request({ type: "close", target: `${session}@${ref}`, ...(force ? { force: true } : {}) }), session);
  }

  reopenSeat(session: string, ref: string): void {
    this.reopens.set(this.request({ type: "reopen", target: `${session}@${ref}` }), session);
  }

  get canReopen(): boolean {
    return this.features.has("reopen");
  }

  launch(role: string, seat: string): void {
    const id = this.request({ type: "launch", role, seat });
    this.launches.set(id, role);
    this.emit({ type: "launch", role, state: "starting", text: `Opening ${role} on ${seat}. Its window opens on the host.` });
  }

  close(): void {
    this.halted = true;
    clearInterval(this.ticker);
    clearInterval(this.refresher);
    clearTimeout(this.redial);
    clearTimeout(this.dialTimer);
    if (typeof window !== "undefined") {
      window.removeEventListener("online", this.wake);
      window.removeEventListener("focus", this.wake);
      document.removeEventListener("visibilitychange", this.wake);
    }
    this.listeners.clear();
    this.socket?.close();
  }

  private browserChannel(): BrowserChannel {
    return {
      watch: (session, size) => {
        this.watching.set(session, size);
        this.request({ type: "browser_watch", session, ...(size ?? {}) });
      },
      unwatch: (session) => {
        this.browsers.delete(session);
        this.watching.delete(session);
        this.request({ type: "browser_unwatch", session });
      },
      control: (session, take, force) => void this.request({ type: "browser_control", session, take, ...(force ? { force } : {}) }),
      input: (session: string, kind: InputKind, params: Record<string, unknown>) => void this.request({ type: "browser_input", session, kind, params }),
      navigate: (session, url) => void this.request({ type: "browser_navigate", session, url }),
    };
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

  private refreshViews(): void {
    if (!this.open || (typeof document !== "undefined" && document.visibilityState !== "visible")) return;
    this.probe();
  }

  private settle(): void {
    const now = Date.now();
    this.watchLink(now);
    let changed = false;
    for (const name of [...this.busy]) {
      if (now - (this.lastOutput.get(name) ?? 0) > BUSY_WINDOW_MS) {
        this.busy.delete(name);
        changed = true;
      }
    }
    if (changed) this.publishSessions();
  }

  private counted(name: string, data: Uint8Array): Uint8Array {
    const got = this.arrived.get(name);
    if (got) {
      got.frames += 1;
      got.bytes += data.length;
    }
    return data;
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
    this.last.sessions = this.sessionViews.map((view) => toSession(view, this.busy.has(view.name), quietSince(view.quiet_seconds, this.viewsAt, this.lastOutput.get(view.name)), this.viewsAt));
    this.emit({ type: "sessions", sessions: this.last.sessions });
  }

  private request(frame: Record<string, unknown>): string {
    const id = `c${++this.nextId}`;
    const line = JSON.stringify({ id, ...frame });
    if (this.open) this.socket?.send(line);
    // After a loss the screen is locked and state is re-attached from what is held.
    else if (!this.answered) this.outbox.push(line);
    return id;
  }

  // Seats and browsers come back on the new connection. A seat with a terminal replays
  // into a cleared screen, and one only watched for activity does not.
  private reattach(): void {
    this.resync = false;
    const live = new Set([...this.sessionViews.map((view) => view.name), ...this.terminalNames]);
    for (const name of this.refs.keys()) {
      if (!live.has(name)) continue;
      const shown = this.attached.get(name);
      if (shown) {
        this.lastPoke.set(name, Date.now());
        this.arrived.set(name, { since: Date.now(), frames: 0, bytes: 0 });
      }
      const box = shown ? { rows: shown.rows, cols: shown.cols, ...(this.ptySize ? { scales: true } : {}) } : {};
      this.request({ type: "attach", session: name, replay: shown !== undefined, ...(shown ? this.resumeFrom(name) : {}), ...box });
    }
    for (const [session, size] of this.watching) this.request({ type: "browser_watch", session, ...(size ?? {}) });
  }

  private receive(frame: Frame): void {
    switch (frame.type) {
      case "welcome": {
        clearTimeout(this.dialTimer);
        if (frame.error || frame.format !== FORMAT) {
          // A daemon that refuses or speaks another format will not change by waiting.
          this.halted = true;
          this.emit({ type: "closed", reason: frame.error ?? `The daemon speaks ${frame.format}.` });
          this.socket?.close();
          return;
        }
        this.open = true;
        this.attempt = 0;
        const redialed = this.answered;
        this.answered = true;
        this.servedBy ??= frame.version;
        this.resync = redialed;
        if (redialed) this.emit({ type: "reconnected", newerBuild: differentBuild(this.servedBy, frame.version) });
        this.browser = frame.features?.includes(BROWSER_FEATURE) ? this.browserChannel() : undefined;
        this.setTyping(parseTyping(frame.typing));
        this.features = new Set(frame.features ?? []);
        this.resumeUnlock(parseTyping(frame.typing));
        this.last.passkey = { type: "passkey", available: this.features.has("passkey") };
        this.emit(this.last.passkey);
        this.last.deviceKey = { type: "device_key", available: this.features.has("device-key") };
        this.emit(this.last.deviceKey);
        const feedback = this.features.has(FEEDBACK_FEATURE);
        this.last.features = { type: "features", terminals: this.features.has("terminals"), ...(this.features.has(PUSH_FEATURE) ? { webPush: true as const } : {}), ...(feedback ? { feedback: true as const } : frame.version ? { daemonVersion: frame.version } : {}) };
        this.emit(this.last.features);
        this.request({ type: "subscribe", channel: "sessions" });
        this.request({ type: "roster" });
        if (this.features.has("mcp-apps") || this.features.has("artifacts")) this.request({ type: "subscribe", channel: "views" });
        // A client that can show a page subscribes. Without a host browser it stays out,
        // but the Android app takes it: no host's browser opens on a phone (COI-3559).
        if (this.features.has(OPEN_URL_FEATURE) && (this.features.has(BROWSER_FEATURE) || inAndroidApp())) this.request({ type: "subscribe", channel: "open_url" });
        // The daemon replays each seat's shelf on subscribe, so a reconnect restores it.
        if (this.features.has("media")) this.request({ type: "subscribe", channel: "media" });
        for (const line of this.outbox.splice(0)) this.socket?.send(this.ptySize ? withScales(line) : line);
        if (this.via && frame.unlock?.daemon_id && frame.unlock.nonce && this.features.has(UNLOCK_FEATURE)) {
          this.channel = { daemonId: frame.unlock.daemon_id, nonce: frame.unlock.nonce, refusals: 0 };
          void this.unlockChannel();
        }
        return;
      }
      case "ask":
        if (frame.ask) this.emit({ type: "ask", ask: toAsk(frame.ask) });
        return;
      case "asked":
        if (frame.ask_id) this.emit({ type: "asked", id: frame.ask_id, outcome: (frame.state ?? "answered") as AskOutcome });
        return;
      case "sessions":
        this.sessionViews = frame.sessions ?? [];
        this.viewsAt = Date.now();
        for (const { name } of frame.terminals ?? []) this.terminalNames.add(name);
        if (this.resync) this.reattach();
        this.monitor();
        this.publishSessions();
        // Absent means none, so a daemon with no shells still reports a list.
        this.last.terminals = { type: "terminals", terminals: (frame.terminals ?? []).map(({ name, label }) => (label ? { id: name, label } : { id: name })) };
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
      case "artifact":
        if (frame.artifact && frame.session) {
          const page = frame.artifact;
          this.emit({
            type: "artifact",
            artifact: { id: page.id, session: frame.session, title: page.title ?? "", url: page.url ?? "", html: page.html ?? "", standing: page.standing === true, waiting: page.waiting === true, seatEnded: page.seat_ended === true },
          });
        }
        return;
      case "artifact_update":
        if (frame.artifact_id) this.emit({ type: "artifact_update", id: frame.artifact_id, ...(frame.waiting === undefined ? {} : { waiting: frame.waiting }), ...(frame.seat_ended === undefined ? {} : { seatEnded: frame.seat_ended }) });
        return;
      case "artifact_closed":
        if (frame.artifact_id) this.emit({ type: "artifact_closed", id: frame.artifact_id });
        return;
      case "view_result":
      case "last_reply":
      case "transcript":
      case "media_data":
      case "image_uploaded":
      case "feedback_image_saved":
      case "feedback_saved":
      case "passkey_enroll_options":
      case "passkey_enrolled":
      case "passkey_assert_options":
      case "passkey_asserted":
      case "push_key":
      case "push_subscribed":
      case "push_unsubscribed":
      case "device_enroll_challenge":
      case "device_enrolled":
      case "device_assert_challenge":
      case "device_asserted":
      case "resumed": {
        // Only unlock replies carry a proof. Absent means no resume window.
        if (UNLOCKED_REPLIES.has(frame.type)) this.resumeToken = resumeProof(frame);
        const call = frame.id ? this.calls.get(frame.id) : undefined;
        if (call && frame.id) {
          this.calls.delete(frame.id);
          this.transcriptIds.delete(frame.id);
          this.feedbackIds.delete(frame.id);
          call.resolve(frame);
        }
        return;
      }
      case "notify": {
        // An older daemon sends none, and one without the feature is not trusted to.
        const note = frame.notify;
        if (this.features.has("notify") && note?.id && note.session && note.title) {
          this.emit({ type: "notify", notify: { id: note.id, session: note.session, role: note.role ?? "", identity: note.identity ?? "", title: note.title, body: note.body ?? "", at: Date.parse(note.at ?? "") || Date.now() } });
        }
        return;
      }
      case "staged":
        if (frame.session && frame.label && typeof frame.body === "string") this.emit({ type: "staged", session: frame.session, label: frame.label, body: frame.body, text: frame.text ?? "" });
        return;
      case "open_url":
        if (frame.session && frame.url) this.emit({ type: "open_url", session: frame.session, url: frame.url });
        return;
      case "media": {
        const item = frame.session ? decodeMedia(frame.session, frame.media) : null;
        if (item) this.emit({ type: "media", item });
        return;
      }
      case "media_closed":
        if (frame.session && frame.media_id) this.emit({ type: "media_closed", session: frame.session, id: frame.media_id });
        return;
      case "typing":
        this.setTyping(parseTyping(frame.typing));
        return;
      case "size":
        if (frame.session && frame.rows && frame.cols) this.emit({ type: "size", sessionId: frame.session, rows: frame.rows, cols: frame.cols });
        return;
      case "roster":
        this.last.roles = { type: "roster", roles: parseRoster(frame.roster) };
        this.emit(this.last.roles);
        return;
      case "output":
        if (!frame.session || !frame.data) return;
        this.noteOutput(frame.session);
        {
          const noted = this.tails.note(frame.session, decode(frame.data), frame.offset);
          if (noted.reset) this.emit({ type: "output_reset", sessionId: frame.session });
          if (noted.data) this.emit({ type: "output", sessionId: frame.session, data: this.counted(frame.session, noted.data) });
        }
        return;
      case "message":
        if (frame.message) this.emit({ type: "message", message: toMessage(frame.message) });
        return;
      case "browser_state": {
        if (!frame.session) return;
        const next = toBrowserState(frame, this.browsers.get(frame.session));
        this.browsers.set(frame.session, next);
        this.emit({ type: "browser", browser: next });
        return;
      }
      case "browser_frame": {
        const next = frame.session ? withBrowserFrame(this.browsers.get(frame.session), frame, Date.now()) : null;
        if (!next) return;
        this.browsers.set(next.session, next);
        this.emit({ type: "browser", browser: next });
        return;
      }
      case "launched":
        if (frame.id) this.launches.delete(frame.id);
        this.emit({ type: "launch", role: frame.role ?? "", state: "started", text: `${frame.role} launched. It appears here once its session starts.` });
        return;
      case "closed": {
        const seat = frame.id ? this.closes.get(frame.id) : undefined;
        if (seat !== undefined && frame.id) {
          this.closes.delete(frame.id);
          this.emit({ type: "seat_closed", session: seat });
        }
        return;
      }
      case "reopened": {
        const seat = frame.id ? this.reopens.get(frame.id) : undefined;
        if (seat !== undefined && frame.id) {
          this.reopens.delete(frame.id);
          this.emit({ type: "seat_reopened", session: seat, role: frame.role ?? "" });
        }
        return;
      }
      case "error": {
        // Code 2: the answer does not fit the ask. Code 3: the ask is gone or settled.
        const role = frame.id ? this.launches.get(frame.id) : undefined;
        const refused = refusalOf(frame);
        if (frame.reason === "unlock_refused" && this.channel) this.channel.refusals += 1;
        this.setTyping(refused);
        const closing = frame.id ? this.closes.get(frame.id) : undefined;
        if (closing !== undefined && frame.id) {
          this.closes.delete(frame.id);
          this.emit({ type: "seat_close_refused", session: closing, text: refused ? typingNotice(refused) : `Not closed. ${frame.error ?? "The daemon would not close it."}`, ...(frame.reason === "unpushed_work" ? { unpushed: true } : {}) });
          return;
        }
        const reopening = frame.id ? this.reopens.get(frame.id) : undefined;
        if (reopening !== undefined && frame.id) {
          this.reopens.delete(frame.id);
          this.emit({ type: "seat_reopen_refused", session: reopening, text: refused ? typingNotice(refused) : `Not reopened. ${frame.error ?? "The daemon would not reopen it."}` });
          return;
        }
        // The lock shows the refusals it knows. Say any other in the daemon's words.
        if (frame.id && this.inputIds.delete(frame.id) && !refused) {
          this.emit({ type: "input_refused", text: `Not sent. ${frame.error ?? "The daemon did not take that input."}` });
          return;
        }
        const call = frame.id ? this.calls.get(frame.id) : undefined;
        if (call && frame.id) {
          this.calls.delete(frame.id);
          const imageWords = refused ? null : imageRefusal(frame.reason);
          if (frame.id && this.feedbackIds.delete(frame.id)) call.reject(new FeedbackRefused(feedbackRefusal(frame.reason) ?? frame.error ?? "The host refused the feedback.", typeof frame.reason === "string" ? frame.reason : undefined));
          else if (frame.id && this.transcriptIds.delete(frame.id) && typeof frame.reason === "string" && Object.hasOwn(REFUSALS, frame.reason)) call.reject(new TranscriptRefused(frame.reason));
          else if (frame.reason === "media_unknown") call.reject(new Error(mediaRefusal(frame.reason)));
          else if (frame.reason === RESUME_ENDED_REASON) call.reject(new ResumeEndedError());
          else if (frame.reason === KEY_UNKNOWN_REASON) call.reject(new DeviceKeyError(KEY_UNKNOWN, KEY_UNKNOWN_REASON));
          else if (imageWords) call.reject(new Error(imageWords));
          else call.reject(new Refusal(refused ? typingNotice(refused) : (frame.error ?? "The daemon refused the request."), refused?.allowed === false ? refused.reason : undefined));
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

  // A redial starts locked. A held proof asks the daemon to restore the unlock before
  // a button shows. Sent ahead of every other frame, which the daemon takes in order.
  private resumeUnlock(typing: Typing | null): void {
    if (!this.resumeToken) return;
    if (!this.features.has(RESUME_FEATURE)) {
      this.resumeToken = undefined;
      return;
    }
    if (!typing || typing.allowed || typing.reason !== "passkey_required") return;
    const token = this.resumeToken;
    this.emit({ type: "resume", state: "resuming" });
    this.exchange({ type: "resume", resume: token }).then(
      // The reply names the next proof, which the receive path already holds.
      () => this.emit({ type: "resume", state: null }),
      (error: unknown) => {
        const ended = error instanceof ResumeEndedError;
        if (ended) this.resumeToken = undefined;
        this.emit({ type: "resume", state: ended ? "ended" : null });
      },
    );
  }

  /** Asks accounts for a token and sends it in an `unlock` frame. */
  private async unlockChannel(): Promise<void> {
    const via = this.via;
    const channel = this.channel;
    const socket = this.socket;
    if (!via || !channel) return;
    let token: string;
    try {
      token = await via.unlock(channel.daemonId, channel.nonce);
    } catch (error) {
      if (socket !== this.socket) return;
      this.setTyping({ allowed: false, reason: "unlock_failed" });
      this.emit({ type: "notice", text: `Accounts did not unlock this connection. ${error instanceof Error ? error.message : ""}`.trim() });
      return;
    }
    // A token is for one channel, so it is never sent down the socket that replaced it.
    if (socket !== this.socket || !this.open || this.channel !== channel) return;
    this.request({ type: "unlock", unlock_token: token });
  }

  /** Unlocks again with a fresh token. Near the nonce limit it redials for a new one. */
  unlockAgain(): void {
    const channel = this.channel;
    if (!this.via || !channel || !this.open) return;
    if (channel.refusals >= UNLOCK_RETRIES) {
      this.socket?.close();
      return;
    }
    void this.unlockChannel();
  }

  private setTyping(typing: Typing | null): void {
    if (!typing) return;
    const could = this.last.typing?.allowed === true;
    this.last.typing = keepStanding(typing, this.last.typing);
    this.emit({ type: "typing", typing: this.last.typing });
    // A client that may not type is refused the media channel, so ask again.
    if (!could && this.last.typing.allowed && this.features.has("media")) this.request({ type: "subscribe", channel: "media" });
  }

  // A listener that throws must not keep the others, a terminal included, from the frame.
  private emit(event: HostEvent): void {
    for (const listener of this.listeners) safely("a host listener", () => listener(event));
  }
}

/** One round trip to learn whether a daemon answers and how many sessions it holds. */
export function probe(url = DEFAULT_DAEMON_URL, timeoutMs = 2000): Promise<number> {
  return new Promise((resolve, reject) => {
    const socket = new WebSocket(url);
    const timer = setTimeout(() => finish(new Error("no answer")), timeoutMs);
    let settled = false;
    function finish(result: number | Error): void {
      // Closing a socket still connecting can raise `error`, which calls this again.
      if (settled) return;
      settled = true;
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
