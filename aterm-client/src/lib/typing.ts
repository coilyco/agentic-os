// Whether this connection may type, as the daemon's typing guard reports it.
// A websocket the guard refuses can still watch. Frames: docs/aterm-daemon.md.

export type Typing = { allowed: true } | { allowed: false; reason: string };

export const TYPING_OPEN: Typing = { allowed: true };

/** The `reason` values the daemon sends when it refuses a guarded frame. */
const REFUSAL_REASONS = new Set(["session_descendant", "peer_unread"]);

/** `welcome.typing`, or null from a daemon that predates the guard. */
export function parseTyping(raw: unknown): Typing | null {
  if (typeof raw !== "object" || raw === null) return null;
  const { allowed, reason } = raw as { allowed?: unknown; reason?: unknown };
  if (allowed === true) return TYPING_OPEN;
  if (allowed !== false) return null;
  return { allowed: false, reason: typeof reason === "string" && reason ? reason : "unspecified" };
}

/** An `error` frame the guard sent. Any other error, reason or not, is no refusal. */
export function refusalOf(frame: { reason?: unknown }): Typing | null {
  return typeof frame.reason === "string" && REFUSAL_REASONS.has(frame.reason) ? { allowed: false, reason: frame.reason } : null;
}

/** What the person reads in place of the composer. Empty when typing is allowed. */
export function typingNotice(typing: Typing): string {
  if (typing.allowed) return "";
  switch (typing.reason) {
    case "session_descendant":
      return "This browser was started by an agent session, so it can watch but not type. Open this page in a browser you started yourself.";
    case "peer_unread":
      return "The daemon could not tell which program opened this page, so it blocks typing to be safe. You can still watch.";
    default:
      return `This connection can watch but not type (${typing.reason}).`;
  }
}
