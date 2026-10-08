// Plain terminals beside a seat, and what the pane shows in each state. The frame is
// COI-2498's, so MockHost scripts the shape proposed there until it is accepted.
import { typingNotice, type Typing } from "./typing";

export interface TerminalEntry {
  id: string;
  /** The seat the client opened it beside. The daemon only echoes it back. */
  label: string | null;
  /** Set once the shell ended. The daemon drops it, the pane keeps saying so. */
  exit: { code: number } | null;
}

/** A terminal as the daemon lists it. */
export interface ListedTerminal {
  id: string;
  label: string | null;
}

/** The daemon's list replaces ours, but an exited shell stays until reopened. */
export function applyList(prev: readonly TerminalEntry[], listed: readonly ListedTerminal[]): TerminalEntry[] {
  const live = listed.map((each): TerminalEntry => ({ ...each, exit: prev.find((entry) => entry.id === each.id)?.exit ?? null }));
  const kept = prev.filter((entry) => entry.exit && !live.some((each) => each.id === entry.id || (entry.label !== null && each.label === entry.label)));
  return [...live, ...kept];
}

/** An exit for a terminal we never heard of is ignored: there is no pane to say it in. */
export function recordExit(prev: readonly TerminalEntry[], id: string, code: number): TerminalEntry[] {
  return prev.map((entry) => (entry.id === id ? { ...entry, exit: { code } } : entry));
}

export function terminalFor(entries: readonly TerminalEntry[], sessionId: string): TerminalEntry | undefined {
  const mine = entries.filter((entry) => entry.label === sessionId);
  return mine.find((entry) => !entry.exit) ?? mine[0];
}

export type PaneState =
  | { kind: "unsupported" }
  | { kind: "waiting" }
  | { kind: "opening" }
  | { kind: "refused"; text: string }
  | { kind: "locked"; reason: string }
  | { kind: "closed" }
  | { kind: "ready" }
  | { kind: "live"; entry: TerminalEntry }
  | { kind: "exited"; entry: TerminalEntry; code: number };

export interface PaneInput {
  /** The host carries terminals at all. A daemon that predates COI-2498 does not. */
  supported: boolean;
  /** The first list has arrived, so absence means none rather than not asked yet. */
  loaded: boolean;
  typing: Typing;
  entry: TerminalEntry | undefined;
  opening: boolean;
  refusal: string | undefined;
  /** Kai closed this seat's terminal, so opening the tab does not reopen it. */
  closed: boolean;
}

/** One state per condition, worst first. */
export function paneState(input: PaneInput): PaneState {
  if (!input.supported) return { kind: "unsupported" };
  if (input.entry) return input.entry.exit ? { kind: "exited", entry: input.entry, code: input.entry.exit.code } : { kind: "live", entry: input.entry };
  if (!input.loaded) return { kind: "waiting" };
  if (input.opening) return { kind: "opening" };
  if (input.refusal) return { kind: "refused", text: input.refusal };
  if (!input.typing.allowed) return { kind: "locked", reason: input.typing.reason };
  return input.closed ? { kind: "closed" } : { kind: "ready" };
}

/** Only a seat nobody has closed a terminal for opens one on its own. */
export function shouldOpen(state: PaneState): boolean {
  return state.kind === "ready";
}

/** How long a spawn may go unanswered before the pane says so. */
export const OPEN_TIMEOUT_MS = 8000;

export const NO_ANSWER = "The host did not answer the request to open a terminal.";

const CLOSED_KEY = "aterm.terminal-closed.v1";
const CLOSED_CAP = 64;

/** Seats whose terminal Kai closed, remembered so a reload does not bring it back. */
export function loadClosed(storage: Pick<Storage, "getItem"> | undefined = safeStorage()): string[] {
  try {
    const parsed: unknown = JSON.parse(storage?.getItem(CLOSED_KEY) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((each): each is string => typeof each === "string") : [];
  } catch {
    return [];
  }
}

export function storeClosed(sessions: readonly string[], storage: Pick<Storage, "setItem"> | undefined = safeStorage()): void {
  try {
    storage?.setItem(CLOSED_KEY, JSON.stringify(sessions.slice(-CLOSED_CAP)));
  } catch {
    // Storage can be off. Closed holds until reload.
  }
}

function safeStorage(): Storage | undefined {
  try {
    return typeof localStorage === "undefined" ? undefined : localStorage;
  } catch {
    return undefined;
  }
}

/** What the pane says when it has no shell to show. */
export function paneText(state: PaneState, seat: string): { title: string; body: string } | null {
  switch (state.kind) {
    case "unsupported":
      return { title: "This host doesn't open terminals yet", body: "Its daemon predates the plain terminal. Once it is updated, a shell opens here, beside " + seat + "." };
    case "waiting":
      return { title: "Asking the host for its terminals", body: "If this stays, the connection is slow or the host stopped answering." };
    case "opening":
      return { title: "Opening a terminal", body: "The host is starting a shell." };
    case "refused":
      return { title: "The host would not open a terminal", body: state.text };
    case "locked":
      return { title: "This browser can watch but not open a terminal", body: typingNotice({ allowed: false, reason: state.reason }) };
    case "closed":
      return { title: "No terminal beside " + seat, body: "You closed it. Open a new one when you want a shell here." };
    case "ready":
      return { title: "No terminal beside " + seat, body: "Open a shell to run a command without leaving this seat." };
    case "live":
    case "exited":
      return null;
  }
}
