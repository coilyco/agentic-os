// The bundled demo host (COI-3315): scripted seats, messages, a choice card, and a
// scripted side terminal, all in this tab. It imports no transport and no shell, so
// it cannot dial anything. Why it is apart from mock-host.ts:
// .agents/skills/tooling-aterm-client/references/demo-mode.md
import rosterJson from "./fixtures/roster.json";
import { envelope } from "./messages";
import { DEMO_FRAMES } from "./fixtures/demo-frames";
import { mediaRefusal } from "./media";
import type { Ask, HostConnection, HostEvent, MediaChannel, PeerMessage, Session, TerminalChannel, TranscriptChannel } from "./protocol";
import { TranscriptRefused } from "./transcript";
import { parseRoster, type Role } from "./roster";
import type { ShellStep } from "./demo-shell";
import {
  afterAnswer,
  afterCancel,
  ASK,
  ASK_SEAT,
  DEMO_PREFIX,
  DEMO_ROLES,
  MEDIA_SEAT,
  messagesFor,
  runDemoLine,
  SEAT_PROMPT,
  SEAT_REPLIES,
  SEATS,
  seatBanner,
  SHELL_PROMPT,
  shellBanner,
  transcriptOf,
} from "./demo-script";
import type { ListedTerminal } from "./terminals";

/** How long a typed line keeps the seat working before it answers. */
const THINK_MS = 1200;
const TURN_TOKENS = 2400;
// Arrow and function keys arrive as escape sequences, which a line editor swallows.
const ESCAPES = /\x1b\[[0-9;?]*[ -/]*[@-~]|\x1bO.|\x1b./g;

function identityOf(roles: Role[], slug: string): string {
  return roles.find((role) => role.slug === slug)?.identity ?? slug;
}

export class DemoHost implements HostConnection {
  readonly demo = true;
  readonly canLaunch = true;
  readonly terminals: TerminalChannel = { open: (label) => this.openShell(label), close: (id) => this.endShell(id, null) };
  // Made-up text, read from memory. A seat the script does not know has no transcript.
  readonly transcripts: TranscriptChannel = {
    read: async (sessionId, options) => {
      const items = transcriptOf(sessionId, this.openedAt);
      if (!items) throw new TranscriptRefused("unsupported_harness");
      const cursor = String(items.length);
      return { session: sessionId, reader: "demo", items: options?.since === cursor ? [] : items, cursor, more: false, updatedAt: this.openedAt };
    },
  };
  // Drawn frames, in memory. Nothing here came from a seat or a disk.
  readonly media: MediaChannel = {
    read: async (item) => {
      const file = DEMO_FRAMES.find((each) => each.id === item.id);
      if (!file) throw new Error(mediaRefusal("media_unknown"));
      return new Blob([Uint8Array.from(atob(file.base64), (char) => char.charCodeAt(0))], { type: file.mime });
    },
  };
  private readonly openedAt = Date.now();
  private listeners = new Set<(event: HostEvent) => void>();
  private timers = new Set<ReturnType<typeof setTimeout>>();
  private roles: Role[];
  private sessions: Session[];
  private buffers = new Map<string, string>();
  private lines = new Map<string, string>();
  private attached = new Set<string>();
  private shells = new Map<string, { label: string }>();
  private asks: Ask[] = [];
  private messages: PeerMessage[] = [];
  private nextShell = 0;
  private replies = 0;
  private started = 0;

  constructor(private readonly delayMs = 700) {
    const everyone = parseRoster(rosterJson);
    this.roles = DEMO_ROLES.map((slug) => everyone.find((role) => role.slug === slug)).filter((role): role is Role => role !== undefined);
    this.sessions = SEATS.map((seat) => ({
      id: seat.id,
      role: seat.role,
      seat: seat.seat,
      identity: identityOf(this.roles, seat.role),
      state: seat.state,
      pending: seat.pending,
      drafting: false,
      paste: true,
      degraded: [],
      context: { tokens: seat.tokens, source: seat.seat },
    }));
    for (const seat of SEATS) this.buffers.set(seat.id, seatBanner() + seat.transcript.join("\r\n") + (seat.id === ASK_SEAT ? "\r\n" : SEAT_PROMPT));
    // The seats show first, then the question and messages arrive, as from a host.
    this.later(delayMs, () => this.deliver(1));
    this.later(delayMs * 2, () => this.deliver(2));
    this.later(delayMs * 2, () => this.ask());
  }

  subscribe(listener: (event: HostEvent) => void): () => void {
    this.listeners.add(listener);
    listener({ type: "roster", roles: this.roles });
    listener({ type: "sessions", sessions: this.sessions });
    for (const message of this.messages) listener({ type: "message", message });
    for (const ask of this.asks) listener({ type: "ask", ask });
    listener({ type: "features", terminals: true });
    for (const file of DEMO_FRAMES) listener({ type: "media", item: { id: file.id, session: MEDIA_SEAT, name: file.name, mime: file.mime, kind: "image", size: Math.floor((file.base64.length * 3) / 4), at: this.openedAt } });
    listener({ type: "terminals", terminals: this.listShells() });
    return () => this.listeners.delete(listener);
  }

  attach(sessionId: string): void {
    this.attached.add(sessionId);
    this.emit({ type: "output", sessionId, data: this.buffers.get(sessionId) ?? "" });
  }

  replay(sessionId: string): void {
    this.emit({ type: "output", sessionId, data: this.buffers.get(sessionId) ?? "" });
  }

  detach(sessionId: string): void {
    this.attached.delete(sessionId);
  }

  resize(): void {}

  input(sessionId: string, data: string): void {
    if (this.shells.has(sessionId)) return this.typeIntoShell(sessionId, data);
    if (!this.sessions.some((each) => each.id === sessionId)) return;
    // The seat waiting on a question takes no free typing, as a harness menu does not.
    if (this.asks.some((each) => each.session === sessionId)) return;
    let line = this.lines.get(sessionId) ?? "";
    for (const char of data.replace(ESCAPES, "")) {
      if (char === "\r" || char === "\n") {
        const said = line.trim();
        line = "";
        if (said) this.turn(sessionId);
        else this.write(sessionId, SEAT_PROMPT);
      } else if (char === "\x7f") {
        if (line) {
          line = line.slice(0, -1);
          this.write(sessionId, "\b \b");
        }
      } else if (char >= " ") {
        line += char;
        this.write(sessionId, char);
      }
    }
    this.lines.set(sessionId, line);
  }

  launch(role: string, seat: string): void {
    const identity = identityOf(this.roles, role);
    const id = `${DEMO_PREFIX}${role}-new-${++this.started}`;
    this.sessions = [...this.sessions, { id, role, seat, identity, state: "idle", pending: 0, drafting: false, paste: true, degraded: [], context: { tokens: 0, source: seat } }];
    this.buffers.set(id, `${seatBanner()}${seat} started for ${identity}.${SEAT_PROMPT}`);
    this.emit({ type: "launch", role, state: "started", text: `${identity} started on the demo host. Nothing real runs.` });
    this.emit({ type: "sessions", sessions: this.sessions });
  }

  answer(askId: string, picks: number[], text?: string): void {
    const ask = this.asks.find((each) => each.id === askId);
    if (!ask) return;
    const labels = picks.map((pick) => ask.options[pick]?.label ?? text ?? "").filter(Boolean);
    this.settle(ask, afterAnswer(labels.length ? labels : [text ?? "no pick"]), "answered");
  }

  cancelAsk(askId: string): void {
    const ask = this.asks.find((each) => each.id === askId);
    if (ask) this.settle(ask, afterCancel(), "cancelled");
  }

  close(): void {
    this.timers.forEach(clearTimeout);
    this.timers.clear();
    this.listeners.clear();
  }

  private settle(ask: Ask, said: string[], outcome: "answered" | "cancelled"): void {
    this.asks = this.asks.filter((each) => each.id !== ask.id);
    this.write(ask.session, `\r\n${said.join("\r\n")}${SEAT_PROMPT}`);
    this.setState(ask.session, "idle", { pending: 0 });
    this.emit({ type: "asked", id: ask.id, outcome });
  }

  private ask(): void {
    this.asks = [ASK];
    this.emit({ type: "ask", ask: ASK });
  }

  private deliver(which: number): void {
    const entry = messagesFor((slug) => identityOf(this.roles, slug)).find((each) => each.after === which);
    if (!entry) return;
    this.messages = [...this.messages, entry.message];
    if (entry.typed && entry.message.session) this.write(entry.message.session, `\r\n${envelope(entry.message.from)} ${entry.typed}${SEAT_PROMPT}`);
    this.emit({ type: "message", message: entry.message });
  }

  /** A line was sent: the seat works for a moment, answers, and its context grows. */
  private turn(sessionId: string): void {
    this.setState(sessionId, "working");
    this.later(THINK_MS, () => {
      const reply = SEAT_REPLIES[this.replies++ % SEAT_REPLIES.length]!;
      this.write(sessionId, `\r\n\r\n${reply.join("\r\n")}${SEAT_PROMPT}`);
      this.setState(sessionId, "idle", { grow: TURN_TOKENS });
    });
  }

  private setState(sessionId: string, state: Session["state"], more: { pending?: number; grow?: number } = {}): void {
    this.sessions = this.sessions.map((each) => {
      if (each.id !== sessionId) return each;
      const context = each.context && more.grow ? { ...each.context, tokens: each.context.tokens + more.grow } : each.context;
      return { ...each, state, pending: more.pending ?? each.pending, ...(context ? { context } : {}) };
    });
    this.emit({ type: "sessions", sessions: this.sessions });
  }

  private listShells(): ListedTerminal[] {
    return [...this.shells].map(([id, { label }]) => ({ id, label }));
  }

  private openShell(label: string): void {
    this.later(250, () => {
      const id = `${DEMO_PREFIX}terminal-${++this.nextShell}`;
      this.shells.set(id, { label });
      this.buffers.set(id, shellBanner());
      this.lines.set(id, "");
      this.emit({ type: "terminals", terminals: this.listShells() });
      this.emit({ type: "terminal_opened", label, id });
    });
  }

  /** A null `code` is a close from a screen: the terminal vanishes with no exit. */
  private endShell(id: string, code: number | null): void {
    if (!this.shells.delete(id)) return;
    this.buffers.delete(id);
    this.lines.delete(id);
    this.attached.delete(id);
    this.emit({ type: "terminals", terminals: this.listShells() });
    if (code !== null) this.emit({ type: "terminal_exit", id, code });
  }

  private typeIntoShell(id: string, data: string): void {
    for (const char of data.replace(ESCAPES, "")) {
      if (!this.shells.has(id)) return;
      const line = this.lines.get(id) ?? "";
      if (char === "\r" || char === "\n") {
        const step: ShellStep = runDemoLine(line);
        this.lines.set(id, "");
        this.write(id, `\r\n${step.output}${step.exit === undefined ? SHELL_PROMPT : ""}`);
        if (step.exit !== undefined) this.endShell(id, step.exit);
      } else if (char === "\x7f") {
        if (line) {
          this.lines.set(id, line.slice(0, -1));
          this.write(id, "\b \b");
        }
      } else if (char >= " ") {
        this.lines.set(id, line + char);
        this.write(id, char);
      }
    }
  }

  private write(id: string, data: string): void {
    this.buffers.set(id, (this.buffers.get(id) ?? "") + data);
    if (this.attached.has(id)) this.emit({ type: "output", sessionId: id, data });
  }

  private later(ms: number, run: () => void): void {
    const timer = setTimeout(() => {
      this.timers.delete(timer);
      run();
    }, ms);
    this.timers.add(timer);
  }

  private emit(event: HostEvent): void {
    for (const listener of this.listeners) listener(event);
  }
}
