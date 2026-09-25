// The demo host: scripted sessions shaped like aterm.daemon.v1, for working on
// the surface with no daemon running. Nothing here reaches a real PTY.
import rosterJson from "./fixtures/roster.json";
import { envelope } from "./messages";
import type { Ask, HostConnection, HostEvent, PeerMessage, Session } from "./protocol";
import { parseRoster } from "./roster";

const DIM = "\x1b[2m";
const RESET = "\x1b[0m";
const PROMPT = "\r\n› ";

const scripts: Record<string, string[]> = {
  "frontend-eng": ["› check the hosts page with the daemon down", `${DIM}● playwright browser_snapshot${RESET}`, "The alert is announced before the host list."],
  "eng-platform": ["› build the websocket session channel", `${DIM}• edit aterm/daemon_ws.go (+84 -0)${RESET}`, `${DIM}• run go test ./aterm/...${RESET}  ok`],
  "sysadmin-senior": ["› how is kai-server doing?", "Root disk is at 74.6%: 364 of 515 GB used.", "Memory is at 53.6% of 33.3 GB."],
  scientist: ["› run the latency sweep", `${DIM}● route 4 of 12, measuring${RESET}`],
};

function session(id: string, role: string, seat: string, identity: string, state: Session["state"], pending = 0): Session {
  return { id, role, seat, identity, state, pending, drafting: false, paste: true };
}

export class MockHost implements HostConnection {
  readonly canLaunch = true;
  private listeners = new Set<(event: HostEvent) => void>();
  private timers: ReturnType<typeof setTimeout>[] = [];
  private roles = parseRoster(rosterJson);
  private sessions: Session[] = [
    session("s-frontend", "frontend-eng", "claude", "Imp-Dragonfly", "idle"),
    session("s-platform", "eng-platform", "codex", "Beetle-Ox", "working"),
    session("s-sysadmin", "sysadmin-senior", "goose", "Turtle-Ox", "idle"),
    session("s-scientist", "scientist", "claude", "Frog-Ox", "working", 1),
    { ...session("s-access", "sysadmin-access", "goose", "Turtle-Ox", "failed"), failure: "goose did not start. Exit 4, goose is not on this host's PATH." },
  ];
  private buffers = new Map<string, string>();
  private attached = new Set<string>();
  private messages: PeerMessage[] = [];
  private asks: Ask[] = [];

  constructor(private readonly delayMs = 1200) {
    for (const each of this.sessions) this.buffers.set(each.id, (scripts[each.role] ?? []).join("\r\n") + PROMPT);
    this.timers.push(setTimeout(() => this.deliverScriptedMessage(), delayMs));
    this.timers.push(setTimeout(() => this.scriptedAsks(), delayMs * 2));
  }

  answer(askId: string, picks: number[], text?: string): void {
    const ask = this.asks.find((each) => each.id === askId);
    if (!ask) return;
    this.asks = this.asks.filter((each) => each.id !== askId);
    const labels = picks.map((pick) => ask.options[pick]?.label ?? text ?? "");
    this.write(ask.session, `\r\n${DIM}● ask_choice answered: ${labels.join(", ")}${RESET}${PROMPT}`);
    this.emit({ type: "asked", id: askId, outcome: "answered" });
  }

  cancelAsk(askId: string): void {
    const ask = this.asks.find((each) => each.id === askId);
    if (!ask) return;
    this.asks = this.asks.filter((each) => each.id !== askId);
    this.write(ask.session, `\r\n${DIM}● ask_choice cancelled${RESET}${PROMPT}`);
    this.emit({ type: "asked", id: askId, outcome: "cancelled" });
  }

  private scriptedAsks(): void {
    this.asks = [
      {
        id: "ask-sweep",
        session: "s-scientist",
        header: "Sweep",
        question: "Which routes should the latency sweep cover first?",
        options: [
          { label: "Local Qwen", description: "Fastest to turn around." },
          { label: "Local Llama", description: "The current default." },
          { label: "Hosted frontier", description: "Costs money per run." },
        ],
        allowOther: true,
        multi: true,
      },
      {
        id: "ask-disk",
        session: "s-sysadmin",
        header: "Disk",
        question: "kai-server root disk is at 74.6%. Where should daemon logs go?",
        options: [
          { label: "/var/log on the data volume", description: "Off the root disk." },
          { label: "Keep them on root", description: "No change, revisit at 80%." },
        ],
        allowOther: true,
        multi: false,
      },
    ];
    for (const ask of this.asks) this.emit({ type: "ask", ask });
  }

  subscribe(listener: (event: HostEvent) => void): () => void {
    this.listeners.add(listener);
    listener({ type: "roster", roles: this.roles });
    listener({ type: "sessions", sessions: this.sessions });
    for (const message of this.messages) listener({ type: "message", message });
    for (const ask of this.asks) listener({ type: "ask", ask });
    return () => this.listeners.delete(listener);
  }

  attach(sessionId: string): void {
    this.attached.add(sessionId);
    this.emit({ type: "output", sessionId, data: this.buffers.get(sessionId) ?? "" });
  }

  detach(sessionId: string): void {
    this.attached.delete(sessionId);
  }

  input(sessionId: string, data: string): void {
    this.write(sessionId, data === "\r" ? PROMPT : data);
  }

  resize(): void {}

  launch(role: string, seat: string): void {
    const identity = this.roles.find((candidate) => candidate.slug === role)?.identity ?? role;
    const id = `s-${role}-${seat}`;
    this.sessions = [...this.sessions.filter((each) => each.role !== role), session(id, role, seat, identity, "idle")];
    this.buffers.set(id, `${DIM}${seat} started for ${identity}${RESET}${PROMPT}`);
    this.emit({ type: "launch", role, state: "started", text: `${role} launched on the demo host.` });
    this.emit({ type: "sessions", sessions: this.sessions });
  }

  close(): void {
    this.timers.forEach(clearTimeout);
    this.listeners.clear();
  }

  private deliverScriptedMessage(): void {
    const message: PeerMessage = {
      id: "m-roster",
      from: { role: "frontend-eng", identity: "Imp-Dragonfly" },
      target: "eng-platform",
      session: "s-platform",
      state: "delivered",
      reason: null,
    };
    this.messages = [message];
    this.write("s-platform", `\r\n${envelope(message.from)} Can the roster ride the socket, not aterm --list --json?${PROMPT}`);
    this.emit({ type: "message", message });
    this.timers.push(
      setTimeout(() => {
        const reply: PeerMessage = {
          id: "m-roster-reply",
          from: { role: "eng-platform", identity: "Beetle-Ox" },
          target: "frontend-eng",
          session: "s-frontend",
          state: "queued",
          reason: "Imp-Dragonfly is mid-turn",
        };
        this.messages = [...this.messages, reply];
        this.emit({ type: "message", message: reply });
      }, this.delayMs),
    );
  }

  private write(sessionId: string, data: string): void {
    this.buffers.set(sessionId, (this.buffers.get(sessionId) ?? "") + data);
    if (this.attached.has(sessionId)) this.emit({ type: "output", sessionId, data });
  }

  private emit(event: HostEvent): void {
    for (const listener of this.listeners) listener(event);
  }
}
