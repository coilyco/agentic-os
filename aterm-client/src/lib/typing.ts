// Whether this connection may type, as the daemon's typing guard reports it.
// A websocket the guard refuses can still watch. Frames: docs/aterm-daemon.md.

/** A remote device's passkey, as the daemon last said: one exists, or none yet. */
export type PasskeyStanding = "enrolled" | "unenrolled";

export type Typing = { allowed: true } | { allowed: false; reason: string; passkey?: PasskeyStanding };

export const TYPING_OPEN: Typing = { allowed: true };

/** The hosted build is the only page whose origin a passkey can belong to. */
const HOSTED_BUILD = import.meta.env.VITE_ATERM_HOSTED === "1";

/** The `reason` values the daemon sends when it refuses a guarded frame. */
const REFUSAL_REASONS = new Set(["session_descendant", "peer_unread", "passkey_required"]);

/** `welcome.typing` or a `typing` push, or null from a daemon that predates the guard. */
export function parseTyping(raw: unknown): Typing | null {
  if (typeof raw !== "object" || raw === null) return null;
  const { allowed, reason, passkey } = raw as { allowed?: unknown; reason?: unknown; passkey?: unknown };
  if (allowed === true) return TYPING_OPEN;
  if (allowed !== false) return null;
  return {
    allowed: false,
    reason: typeof reason === "string" && reason ? reason : "unspecified",
    ...(passkey === "enrolled" || passkey === "unenrolled" ? { passkey } : {}),
  };
}

/** An `error` frame the guard sent. Any other error, reason or not, is no refusal. */
export function refusalOf(frame: { reason?: unknown }): Typing | null {
  return typeof frame.reason === "string" && REFUSAL_REASONS.has(frame.reason) ? { allowed: false, reason: frame.reason } : null;
}

/** A refusal frame names no passkey standing, so keep the one the welcome gave. */
export function keepStanding(next: Typing, previous: Typing | undefined): Typing {
  if (next.allowed || next.passkey || !previous || previous.allowed || previous.reason !== next.reason || !previous.passkey) return next;
  return { ...next, passkey: previous.passkey };
}

/** True while a passkey can lift the lock. A refused `cancel_ask` stays disabled then. */
export function needsPasskey(typing: Typing): boolean {
  return !typing.allowed && typing.reason === "passkey_required";
}

/** What the person reads in place of the composer. Empty when typing is allowed. */
export function typingNotice(typing: Typing, hosted = HOSTED_BUILD): string {
  if (typing.allowed) return "";
  switch (typing.reason) {
    case "session_descendant":
      return "This browser was started by an agent session, so it can watch but not type. Open this page in a browser you started yourself.";
    case "peer_unread":
      return "The daemon could not tell which program opened this page, so it blocks typing to be safe. You can still watch.";
    case "passkey_required":
      return hosted
        ? "This device is locked. It can watch, and types after you unlock it with a passkey."
        : "This device is locked, and passkeys only work from the hosted client at coilyco.dev. Open it there, add this host, and unlock it.";
    default:
      return `This connection can watch but not type (${typing.reason}).`;
  }
}
