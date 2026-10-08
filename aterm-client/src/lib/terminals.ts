// Plain terminals beside a seat, and what the pane shows in each state. The frame is
// COI-2498's, in the aterm daemon page. The daemon never says which seat one sits beside.
import { typingNotice, type Typing } from "./typing";

export interface TerminalEntry {
  id: string;
  /** The seat this client opened it beside, null for one it did not open. */
  label: string | null;
  /** Set once the shell ended. A null code is an end whose exit frame has not come. */
  exit: { code: number | null } | null;
}

/** A terminal as the daemon lists it. */
export interface ListedTerminal {
  id: string;
}

/** Terminal id to the seat it was opened beside, remembered on this device. */
export type Seats = Record<string, string>;

// The daemon's list replaces ours. A shell that drops out ended, unless it is in
// `dismissed`, and stays ended until reopened, whether the exit or the list came first.
export function applyList(prev: readonly TerminalEntry[], listed: readonly ListedTerminal[], seats: Seats, dismissed: ReadonlySet<string> = new Set()): TerminalEntry[] {
  const live = listed.map(({ id }): TerminalEntry => ({ id, label: seats[id] ?? null, exit: prev.find((entry) => entry.id === id)?.exit ?? null }));
  const ended = prev
    .filter((entry) => !live.some((each) => each.id === entry.id) && !dismissed.has(entry.id))
    .map((entry): TerminalEntry => ({ ...entry, exit: entry.exit ?? { code: null } }))
    .filter((entry) => entry.label === null || !live.some((each) => each.label === entry.label));
  return [...live, ...ended];
}

/** An exit for a terminal we never heard of is ignored: there is no pane to say it in. */
export function recordExit(prev: readonly TerminalEntry[], id: string, code: number): TerminalEntry[] {
  return prev.map((entry) => (entry.id === id ? { ...entry, exit: { code } } : entry));
}

/** The daemon says which shell it opened after the list may already show it. */
export function claim(prev: readonly TerminalEntry[], id: string, label: string): TerminalEntry[] {
  return prev.map((entry) => (entry.id === id ? { ...entry, label } : entry));
}

export function exitText(code: number | null): string {
  return code === null ? "The shell ended." : `The shell exited with code ${code}.`;
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
  | { kind: "exited"; entry: TerminalEntry; code: number | null };

export type Support = "unknown" | "yes" | "no";

export interface PaneInput {
  /** The welcome's feature list. A daemon that predates COI-2498 lacks "terminals". */
  support: Support;
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
  if (input.support === "no") return { kind: "unsupported" };
  if (input.entry) return input.entry.exit ? { kind: "exited", entry: input.entry, code: input.entry.exit.code } : { kind: "live", entry: input.entry };
  if (input.support === "unknown" || !input.loaded) return { kind: "waiting" };
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
const SEATS_KEY = "aterm.terminal-seats.v1";
const CAP = 64;

/** Which seat each terminal sits beside. Only this browser knows it. */
export function loadSeats(storage: Pick<Storage, "getItem"> | undefined = safeStorage()): Seats {
  try {
    const parsed: unknown = JSON.parse(storage?.getItem(SEATS_KEY) ?? "{}");
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return {};
    return Object.fromEntries(Object.entries(parsed).filter((entry): entry is [string, string] => typeof entry[1] === "string"));
  } catch {
    return {};
  }
}

export function storeSeats(seats: Seats, storage: Pick<Storage, "setItem"> | undefined = safeStorage()): void {
  try {
    storage?.setItem(SEATS_KEY, JSON.stringify(Object.fromEntries(Object.entries(seats).slice(-CAP))));
  } catch {
    // Storage can be off. The seat holds until reload.
  }
}

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
    storage?.setItem(CLOSED_KEY, JSON.stringify(sessions.slice(-CAP)));
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
