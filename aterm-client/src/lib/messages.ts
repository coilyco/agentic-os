import type { MessageState, PeerMessage } from "./protocol";

/** The line the daemon stamps ahead of every peer message. Human input carries none. */
export function envelope(from: PeerMessage["from"]): string {
  return `[from ${from.role} ${from.identity}]`;
}

/** Lines that open a known peer message. Why matching is safe: docs/architecture.md. */
export function envelopeRows(lines: readonly string[], messages: readonly PeerMessage[]): Map<number, PeerMessage> {
  const byEnvelope = new Map(messages.map((message) => [envelope(message.from), message]));
  const rows = new Map<number, PeerMessage>();
  lines.forEach((line, row) => {
    const match = byEnvelope.get(line.trimEnd());
    if (match) rows.set(row, match);
  });
  return rows;
}

export function upsertMessage(list: readonly PeerMessage[], next: PeerMessage): PeerMessage[] {
  const index = list.findIndex((message) => message.id === next.id);
  if (index === -1) return [...list, next];
  return list.map((message, at) => (at === index ? next : message));
}

export const stateLabel: Record<MessageState, string> = {
  delivered: "typed in",
  queued: "queued",
  held: "held",
  bounced: "bounced",
};
