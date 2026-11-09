// A seat's last reply, pinned under the message box on a phone (COI-3253).
import { ageText } from "./age";

/** "3m ago", "just now" under ten seconds, and empty when the daemon gave no time. */
export function replyAge(at: number | undefined, now: number): string {
  if (at === undefined) return "";
  const text = ageText(Math.max(0, (now - at) / 1000));
  return text ? `${text} ago` : "just now";
}

/** The daemon's first line with its whitespace folded into one sentence. */
export function pinnedLine(first: string | undefined): string {
  return (first ?? "").replace(/\s+/g, " ").trim();
}

/** The whole reply is fetched once per reply: a new `last_reply_at` is a new reply. */
export function replyKey(sessionId: string, at: number | undefined, line: string): string {
  return `${sessionId}:${at ?? "?"}:${line}`;
}
