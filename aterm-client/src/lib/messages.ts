import type { MessageState, PeerMessage } from "./protocol";

/** The prefix the daemon stamps ahead of every peer message. Human input carries none. */
export function envelope(from: PeerMessage["from"]): string {
  return `[from ${from.role} ${from.identity}]`;
}

/** Splits the daemon's `from`, which is `<role> <identity>`. */
export function parseFrom(from: string): PeerMessage["from"] {
  const space = from.indexOf(" ");
  return space === -1 ? { role: from, identity: from } : { role: from.slice(0, space), identity: from.slice(space + 1) };
}

/** Lines holding a stamped envelope from a known sender. See docs/architecture.md. */
export function envelopeRows(lines: readonly string[], messages: readonly PeerMessage[]): Map<number, PeerMessage> {
  const senders = new Map(messages.map((message) => [envelope(message.from), message]));
  const rows = new Map<number, PeerMessage>();
  lines.forEach((line, row) => {
    for (const [stamp, message] of senders) {
      const at = line.indexOf(stamp);
      if (at !== -1 && line[at - 1] !== "\\") {
        rows.set(row, message);
        return;
      }
    }
  });
  return rows;
}

export function upsertMessage(list: readonly PeerMessage[], next: PeerMessage): PeerMessage[] {
  const index = list.findIndex((message) => message.id === next.id);
  if (index === -1) return [...list, next];
  return list.map((message, at) => (at === index ? next : message));
}

export const stateLabel: Record<MessageState, string> = {
  queued: "queued",
  held: "held",
  launching: "launching",
  delivered: "typed in",
  failed: "failed",
};
