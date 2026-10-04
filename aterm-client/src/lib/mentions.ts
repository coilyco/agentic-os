// `@` completion in the composer: which live sessions a typed prefix offers, and the
// edit that inserts one. It reads the session list the sidebar already holds, so the
// daemon has no verb for it. The shell does the same from `aterm agents`.
import type { Session } from "./protocol";

export interface Mention {
  /** Index of the `@`. */
  start: number;
  /** End of the token, past the caret when the caret sits inside it. */
  end: number;
  /** What was typed between the `@` and the caret. */
  query: string;
}

/** The `@token` the caret is in, or null. The `@` must open the text or follow
 * whitespace, so `a@b.example` never triggers. */
export function findMention(text: string, caret: number): Mention | null {
  const match = /(^|\s)@([^\s@]*)$/.exec(text.slice(0, caret));
  if (!match) return null;
  const query = match[2] ?? "";
  const rest = /^[^\s]*/.exec(text.slice(caret));
  return { start: caret - query.length - 1, end: caret + (rest ? rest[0].length : 0), query };
}

/** Live sessions the prefix offers, by session name or identity, in name order. A
 * session that failed to start is not live, so it is never offered. */
export function mentionMatches(sessions: readonly Session[], query: string): Session[] {
  const prefix = query.toLowerCase();
  return sessions
    .filter((session) => session.state !== "failed")
    .filter((session) => session.id.toLowerCase().startsWith(prefix) || session.identity.toLowerCase().startsWith(prefix))
    .sort((a, b) => a.id.localeCompare(b.id));
}

/** Replaces the token with the full session name, no `@`, and a space after it
 * unless whitespace already follows. The caret lands after that space. */
export function insertMention(text: string, mention: Mention, name: string): { text: string; caret: number } {
  const tail = text.slice(mention.end);
  const joined = /^\s/.test(tail);
  const head = `${text.slice(0, mention.start)}${name}${joined ? "" : " "}`;
  return { text: head + tail, caret: head.length + (joined ? 1 : 0) };
}
