// The client's view of a host. `DaemonHost` maps aterm.daemon.v1 onto it,
// and `MockHost` scripts it for the demo. Why: the architecture reference.
import type { ToolResult, View } from "./mcp-apps";
import type { Role } from "./roster";
import type { InputKind, SharedBrowser } from "./screencast";

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
}

export type SessionState = "idle" | "working" | "failed";

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
  | { type: "closed"; reason: string }
  | { type: "view"; view: View }
  | { type: "view_update"; id: string; toolResult?: ToolResult; cancelled?: string }
  | { type: "view_closed"; id: string }
  | { type: "browser"; browser: SharedBrowser };

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

export interface HostConnection {
  readonly canLaunch: boolean;
  readonly views?: ViewChannel;
  readonly browser?: BrowserChannel;
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
