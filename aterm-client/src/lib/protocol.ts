// The client's view of a host. `DaemonHost` maps aterm.daemon.v1 onto it,
// and `MockHost` scripts it for the demo. Why: the architecture reference.
import type { ToolResult, View } from "./mcp-apps";
import type { Role } from "./roster";
import type { InputKind, SharedBrowser } from "./screencast";
import type { ListedTerminal } from "./terminals";
import type { Typing } from "./typing";

export type HostKind = "daemon" | "demo";

export type HostStatus =
  | { kind: "checking" }
  | { kind: "online"; sessionCount: number }
  | { kind: "unreachable"; reason: string };

export interface Host {
  id: string;
  label: string;
  address: string;
  kind: HostKind;
  status: HostStatus;
  /** Found by a daemon on the tailnet, not typed here. Never stored on this device. */
  found?: boolean;
}

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
  /** Not yet ready for input, so its terminal may have nothing to draw. */
  starting?: boolean;
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
}

export type AskOutcome = "answered" | "cancelled" | "timed_out";

export type HostEvent =
  | { type: "roster"; roles: Role[] }
  | { type: "sessions"; sessions: Session[] }
  | { type: "output"; sessionId: string; data: string | Uint8Array }
  | { type: "message"; message: PeerMessage }
  | { type: "notice"; text: string }
  | { type: "ask"; ask: Ask }
  | { type: "asked"; id: string; outcome: AskOutcome }
  | { type: "launch"; role: string; state: LaunchState; text: string }
  | { type: "typing"; typing: Typing }
  | { type: "closed"; reason: string }
  | { type: "view"; view: View }
  | { type: "view_update"; id: string; toolResult?: ToolResult; cancelled?: string }
  | { type: "view_closed"; id: string }
  | { type: "browser"; browser: SharedBrowser }
  | { type: "features"; terminals: boolean }
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
  watch(sessionId: string): void;
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

export interface HostConnection {
  readonly canLaunch: boolean;
  readonly views?: ViewChannel;
  readonly browser?: BrowserChannel;
  readonly terminals?: TerminalChannel;
  subscribe(listener: (event: HostEvent) => void): () => void;
  attach(sessionId: string, rows: number, cols: number): void;
  detach(sessionId: string): void;
  input(sessionId: string, data: string): void;
  resize(sessionId: string, rows: number, cols: number): void;
  launch(role: string, seat: string): void;
  answer(askId: string, picks: number[], text?: string): void;
  cancelAsk(askId: string): void;
  close(): void;
}
