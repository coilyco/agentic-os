// Stands in for the host daemon until teable:coilyco/agentic-os#8219 lands.
// Everything here is scripted, and nothing reaches a real PTY.
import rosterJson from "./fixtures/roster.json";
import { envelope } from "./messages";
import type { Host, HostConnection, HostEvent, PeerMessage, Session } from "./protocol";
import { parseRoster } from "./roster";

export const mockHosts: Host[] = [
  { id: "local", label: "this Mac", address: "localhost", status: { kind: "online", sessionCount: 4 } },
  { id: "kai-server", label: "kai-server", address: "tailnet", status: { kind: "online", sessionCount: 1 } },
  { id: "ser8", label: "ser8", address: "tailnet", status: { kind: "unreachable", lastSeen: null } },
  { id: "dev-base", label: "dev-base container", address: "container", status: { kind: "auth-required" } },
];

const DIM = "\x1b[2m";
const RESET = "\x1b[0m";
const PROMPT = "\r\n› ";

const scripts: Record<string, string[]> = {
  "frontend-eng": ["› check the hosts page with ser8 down", `${DIM}● playwright browser_snapshot${RESET}`, "The alert is announced before the host list."],
  "eng-platform": ["› build the websocket session channel", `${DIM}• edit aterm/daemon/pty.go (+84 -0)${RESET}`, `${DIM}• run go test ./aterm/...${RESET}  ok`],
  "sysadmin-senior": ["› how is kai-server doing?", "Root disk is at 74.6%: 364 of 515 GB used.", "Memory is at 53.6% of 33.3 GB."],
  scientist: ["› run the latency sweep", `${DIM}● route 4 of 12, measuring${RESET}`],
};

export class MockHost implements HostConnection {
  private listeners = new Set<(event: HostEvent) => void>();
  private timers: ReturnType<typeof setTimeout>[] = [];
  private roles = parseRoster(rosterJson);
  private sessions: Session[] = [
    { id: "s-frontend", role: "frontend-eng", seat: "claude", identity: "Imp-Dragonfly", state: "idle" },
    { id: "s-platform", role: "eng-platform", seat: "codex", identity: "Beetle-Ox", state: "working" },
    { id: "s-sysadmin", role: "sysadmin-senior", seat: "goose", identity: "Turtle-Ox", state: "idle" },
    { id: "s-scientist", role: "scientist", seat: "claude", identity: "Frog-Ox", state: "working" },
    { id: "s-access", role: "sysadmin-access", seat: "goose", identity: "Turtle-Ox", state: "failed", failure: "goose did not start. Exit 4, goose is not on this host's PATH." },
  ];
  private buffers = new Map<string, string>();
  private messages: PeerMessage[] = [];

  constructor(private readonly delayMs = 1200) {
    for (const session of this.sessions) {
      this.buffers.set(session.id, (scripts[session.role] ?? []).join("\r\n") + PROMPT);
    }
    this.timers.push(setTimeout(() => this.deliverScriptedMessage(), delayMs));
  }

  subscribe(listener: (event: HostEvent) => void): () => void {
    this.listeners.add(listener);
    listener({ type: "roster", roles: this.roles });
    listener({ type: "sessions", sessions: this.sessions });
    for (const [sessionId, data] of this.buffers) listener({ type: "output", sessionId, data });
    for (const message of this.messages) listener({ type: "message", message });
    return () => this.listeners.delete(listener);
  }

  input(sessionId: string, data: string): void {
    this.write(sessionId, data === "\r" ? PROMPT : data);
  }

  launch(role: string, seat: string): void {
    const identity = this.roles.find((candidate) => candidate.slug === role)?.identity ?? role;
    const id = `s-${role}-${seat}`;
    this.sessions = [
      ...this.sessions.filter((session) => session.role !== role),
      { id, role, seat, identity, state: "idle" },
    ];
    this.buffers.set(id, "");
    this.emit({ type: "sessions", sessions: this.sessions });
    this.write(id, `${DIM}${seat} started for ${identity}${RESET}${PROMPT}`);
  }

  close(): void {
    this.timers.forEach(clearTimeout);
    this.listeners.clear();
  }

  private deliverScriptedMessage(): void {
    const message: PeerMessage = {
      id: "m-roster-channel",
      from: { role: "frontend-eng", identity: "Imp-Dragonfly" },
      to: { role: "eng-platform", identity: "Beetle-Ox" },
      body: "The client needs the roster pushed over the socket.\r\nCan milestone one carry a roster channel?",
      state: "delivered",
    };
    this.messages = [message];
    this.write("s-platform", `\r\n${envelope(message.from)}\r\n${message.body}${PROMPT}`);
    this.emit({ type: "message", message });
    this.timers.push(
      setTimeout(() => {
        const reply: PeerMessage = {
          id: "m-roster-reply",
          from: { role: "eng-platform", identity: "Beetle-Ox" },
          to: { role: "frontend-eng", identity: "Imp-Dragonfly" },
          body: "yes, roster rides channel 2",
          state: "queued",
        };
        this.messages = [...this.messages, reply];
        this.emit({ type: "message", message: reply });
      }, this.delayMs),
    );
  }

  private write(sessionId: string, data: string): void {
    this.buffers.set(sessionId, (this.buffers.get(sessionId) ?? "") + data);
    this.emit({ type: "output", sessionId, data });
  }

  private emit(event: HostEvent): void {
    for (const listener of this.listeners) listener(event);
  }
}
