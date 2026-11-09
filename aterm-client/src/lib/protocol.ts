// The client's view of a host. `DaemonHost` maps aterm.daemon.v1 onto it,
// and `MockHost` scripts it for the demo. Why: the architecture reference.
import type { KeyStatus } from "./device-key";
import type { ToolResult, View } from "./mcp-apps";
import type { ResumeState } from "./resume";
import type { Role } from "./roster";
import type { Layer } from "./reach";
import type { Activity } from "./seat-activity";
import type { InputKind, SharedBrowser } from "./screencast";
import type { ListedTerminal } from "./terminals";
import type { Typing } from "./typing";

export type HostKind = "daemon" | "demo";

export type HostStatus =
  | { kind: "checking" }
  | { kind: "online"; sessionCount: number }
  | { kind: "unreachable"; reason: string; layer?: Layer };

export interface Host {
  id: string;
  label: string;
  address: string;
  kind: HostKind;
  status: HostStatus;
  /** Found by a tailnet daemon, not typed here. Not stored until saved. */
  found?: boolean;
  /** It answered this page load: silence is a restart, and no listing change drops it. */
  answered?: boolean;
}

/** The attached host's link: live, lost and being redialed, or just back. */
export type Link =
  | { state: "live" }
  | { state: "reconnecting"; attempt: number; retryAt: number | null }
  // The hour of automatic tries is spent. A return to the page, or Retry, dials again.
  | { state: "resting" }
  | { state: "restored" };

export type SessionState = "idle" | "working" | "failed";

/** How full a seat's context is, from whichever source its harness has. */
export interface ContextReading {
  tokens: number;
  /** The model's window, only where the source knows it. A percent needs it. */
  window?: number;
  /** Where the daemon read it: `claude` or `codex` transcript, or `proxy`. */
  source: string;
}

export interface Session {
  id: string;
  role: string;
  seat: string;
  identity: string;
  state: SessionState;
  pending: number;
  drafting: boolean;
  /** The program asked for bracketed paste, so text should arrive as one paste. */
  paste: boolean;
  /** Startup steps agent-compose launched without, named as the daemon names them. */
  degraded: string[];
  /** Absent until a source has read one, and from a daemon that predates it. */
  context?: ContextReading;
  /** First line of its last finished reply. Absent until read or from an old daemon. */
  lastReply?: string;
  /** When it last wrote, in ms. Absent from a daemon that predates `quiet_seconds`. */
  quietSince?: number;
  /** What the seat says it is doing, and when it set that. Absent until it sets one. */
  activity?: Activity;
  /** Not yet ready for input, so its terminal may have nothing to draw. */
  starting?: boolean;
  /** What the daemon read off the screen. `state` above also folds in recent output. */
  screen?: "starting" | "busy" | "idle" | "prompt";
  /** Background tasks the seat has running. Absent from a daemon without the field. */
  background?: number;
  /** Names this session and not its name, which is reused. Close takes `name@ref`. */
  ref?: string;
  failure?: string;
}

export type LaunchState = "starting" | "started" | "failed";

export type MessageState = "queued" | "held" | "launching" | "delivered" | "failed";

export interface PeerMessage {
  id: string;
  from: { role: string; identity: string };
  target: string;
  session: string | null;
  state: MessageState;
  reason: string | null;
  /** What the sender wrote. Absent from a daemon that does not send it (COI-3362). */
  text?: string | null;
  /** The host sent only the head of a longer message. */
  textCut?: boolean;
}

/** A seat's `ask_choice` call, waiting on a person. */
export interface Ask {
  id: string;
  session: string;
  header: string;
  question: string;
  options: { label: string; description: string }[];
  allowOther: boolean;
  multi: boolean;
  /** A worker's question relayed to a director, so `session` is the asker (COI-3241). */
  relay?: boolean;
  /** The session whose pane shows the card. Absent when no director was up. */
  cardSeat?: string;
}

export type AskOutcome = "answered" | "cancelled" | "timed_out";

/** A page a seat put on Kai's desk. Only whether it waits on her is read here. */
export interface SeatArtifact {
  id: string;
  /** The posting seat. It stays after the seat ends, so see `seatEnded`. */
  session: string;
  waiting: boolean;
  seatEnded: boolean;
}

export type HostEvent =
  | { type: "roster"; roles: Role[] }
  | { type: "sessions"; sessions: Session[] }
  | { type: "output"; sessionId: string; data: string | Uint8Array }
  | { type: "message"; message: PeerMessage }
  | { type: "notice"; text: string }
  // A seat asked to open a page. Its name is the label, and the daemon names no pane.
  | { type: "open_url"; session: string; url: string }
  // Typed input not taken for a reason the lock does not show, or lost to a dead socket.
  | { type: "input_refused"; text: string }
  | { type: "ask"; ask: Ask }
  | { type: "asked"; id: string; outcome: AskOutcome }
  | { type: "artifact"; artifact: SeatArtifact }
  | { type: "artifact_update"; id: string; waiting?: boolean; seatEnded?: boolean }
  | { type: "artifact_closed"; id: string }
  | { type: "seat_closed"; session: string }
  | { type: "seat_close_refused"; session: string; text: string }
  | { type: "launch"; role: string; state: LaunchState; text: string }
  | { type: "typing"; typing: Typing }
  | { type: "closed"; reason: string }
  // The daemon stopped. `retryAt` is the next dial's time, null while one is out.
  | { type: "reconnecting"; attempt: number; retryAt: number | null }
  // The hour of tries ended with no answer. Nothing is scheduled until a wake or Retry.
  | { type: "resting" }
  // It answered again. `newerBuild` means it is not the build that served this page.
  | { type: "reconnected"; newerBuild: boolean }
  // A held unlock proof is out (`resuming`), the daemon said it ended, or neither (null).
  | { type: "resume"; state: ResumeState | null }
  | { type: "view"; view: View }
  | { type: "view_update"; id: string; toolResult?: ToolResult; cancelled?: string }
  | { type: "view_closed"; id: string }
  | { type: "browser"; browser: SharedBrowser }
  | { type: "features"; terminals: boolean }
  // Whether this daemon runs a passkey ceremony. Apart from `features`, which tests pin.
  | { type: "passkey"; available: boolean }
  // Whether the daemon lists `device-key`. The page also needs the app's plugin.
  | { type: "device_key"; available: boolean }
  // The PTY's effective size, which a client sets its terminal to exactly.
  | { type: "size"; sessionId: string; rows: number; cols: number }
  | { type: "terminals"; terminals: ListedTerminal[] }
  | { type: "terminal_opened"; label: string; id: string }
  | { type: "terminal_exit"; id: string; code: number }
  | { type: "terminal_refused"; label: string; text: string };

/** MCP Apps views. Only a daemon that forwards them has this. */
export interface ViewChannel {
  call(viewId: string, method: "tools/call" | "resources/read", params: Record<string, unknown>): Promise<unknown>;
  close(viewId: string): void;
}

/** The seat's shared browser. Only a daemon that streams one has this. */
export interface BrowserChannel {
  /** `size` is the pane in CSS pixels, which bounds the screencast the host sends. */
  watch(sessionId: string, size?: { width: number; height: number }): void;
  unwatch(sessionId: string): void;
  /** `force` takes control from another screen that holds it. */
  control(sessionId: string, take: boolean, force?: boolean): void;
  input(sessionId: string, kind: InputKind, params: Record<string, unknown>): void;
  navigate(sessionId: string, url: string): void;
}

/** Plain shells beside a seat (COI-2498). Only a daemon with that kind has this. */
export interface TerminalChannel {
  /** `label` is the seat's session name, which the host echoes back in its list. */
  open(label: string): void;
  close(id: string): void;
}

/** Images for a seat (COI-3240). No daemon has one yet (COI-3239). */
export interface ImageChannel {
  /** The host's own limit in bytes, so the client never guesses one. */
  maxBytes: number;
  /** Puts the image on the seat's host and resolves its path. Rejects in words. */
  upload(sessionId: string, image: Blob): Promise<string>;
}

/** The screen as the client saw it. Display text, never trusted. */
export interface FeedbackContext {
  view: string;
  host: string;
  viewport: { width: number; height: number; ratio: number };
  build: string;
  /** The Android app, not a browser. */
  app: boolean;
  at: string;
}

export interface FeedbackReport {
  text: string;
  context: FeedbackContext;
  images: Blob[];
}

/** A note and screenshots kept as a host record. Needs `feedback` (COI-3345). */
export interface FeedbackChannel {
  /** The host's limit per screenshot in bytes. */
  maxBytes: number;
  /** Uploads each image, then the note. Resolves the record's id. Rejects in words. */
  send(report: FeedbackReport, progress?: (done: number, total: number) => void): Promise<string>;
}

/** A remote device's passkey. Only a daemon that lists `passkey` has one. */
export interface PasskeyChannel {
  /** Spends the one-time code, makes a passkey, and unlocks. Rejects in words. */
  enroll(code: string): Promise<void>;
  /** Asserts the enrolled passkey and unlocks the connection. Rejects in words. */
  assert(): Promise<void>;
}

/** The app's phone-held key. Needs a daemon with `device-key` and the plugin. */
export interface DeviceKeyChannel {
  /** The phone's own state, answered without a prompt. */
  status(): Promise<KeyStatus>;
  /** Spends the one-time code, makes a Keystore key, and unlocks. Rejects in words. */
  enroll(code: string): Promise<void>;
  /** Asks for a fingerprint, signs the challenge, and unlocks. Rejects in words. */
  assert(): Promise<void>;
}

export interface HostConnection {
  readonly canLaunch: boolean;
  readonly passkey?: PasskeyChannel;
  readonly deviceKey?: DeviceKeyChannel;
  readonly views?: ViewChannel;
  readonly browser?: BrowserChannel;
  readonly terminals?: TerminalChannel;
  readonly images?: ImageChannel;
  readonly feedback?: FeedbackChannel;
  subscribe(listener: (event: HostEvent) => void): () => void;
  /** Dial now, skipping the backoff. Only a connection that redials has it. */
  retry?(): void;
  /** Ends a seat by its name and ref, which a reused name cannot confuse (COI-3403). */
  closeSeat?(session: string, ref: string): void;
  attach(sessionId: string, rows: number, cols: number): void;
  /** Asks again for a seat's screen when its replay never drew. No new reference. */
  replay(sessionId: string): void;
  detach(sessionId: string): void;
  input(sessionId: string, data: string): void;
  /** `rows` and `cols` are this client's box. A `ptySize` daemon answers `size`. */
  resize(sessionId: string, rows: number, cols: number): void;
  /** The daemon sizes the PTY itself and sends `size`, so the terminal follows it. */
  readonly ptySize?: boolean;
  launch(role: string, seat: string): void;
  answer(askId: string, picks: number[], text?: string): void;
  cancelAsk(askId: string): void;
  close(): void;
}
