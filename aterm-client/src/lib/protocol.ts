// Client-side draft of the daemon contract (teable:coilyco/agentic-os#8220).
// The daemon's published schema replaces this file once it exists.
import type { Role } from "./roster";

export type HostStatus =
  | { kind: "online"; sessionCount: number }
  | { kind: "unreachable"; lastSeen: string | null }
  | { kind: "auth-required" };

export interface Host {
  id: string;
  label: string;
  address: string;
  status: HostStatus;
}

export type SessionState = "idle" | "working" | "failed";

export interface Session {
  id: string;
  role: string;
  seat: string;
  identity: string;
  state: SessionState;
  failure?: string;
}

export type MessageState = "delivered" | "queued" | "held" | "bounced";

export interface PeerMessage {
  id: string;
  from: { role: string; identity: string };
  to: { role: string; identity: string | null };
  body: string;
  state: MessageState;
}

export type HostEvent =
  | { type: "roster"; roles: Role[] }
  | { type: "sessions"; sessions: Session[] }
  | { type: "output"; sessionId: string; data: string }
  | { type: "message"; message: PeerMessage };

export interface HostConnection {
  subscribe(listener: (event: HostEvent) => void): () => void;
  input(sessionId: string, data: string): void;
  launch(role: string, seat: string): void;
  close(): void;
}
