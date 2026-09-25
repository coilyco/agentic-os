// The client's view of a host. `DaemonHost` maps aterm.daemon.v1 onto it,
// and `MockHost` scripts it for the demo. Why: docs/architecture.md.
import type { Role } from "./roster";

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
  | { type: "closed"; reason: string };

export interface HostConnection {
  readonly canLaunch: boolean;
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
