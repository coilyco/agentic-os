// What hovering a glowing seat in the sidebar says, without opening it.

import type { Ask } from "./protocol";

export interface Preview {
  identity: string;
  kind: "asking" | "done";
  /** What the seat is doing to you, in the sidebar's own words. */
  heading: string;
  /** The question it asks, or for a finished seat the first line of its last reply. */
  detail: string | null;
}

const DETAIL_CHARS = 280;

const oneLine = (text: string): string => {
  const line = text.trim().replace(/\s+/g, " ");
  return line.length > DETAIL_CHARS ? `${line.slice(0, DETAIL_CHARS).trimEnd()}…` : line;
};

export function previewFor(seat: { identity: string; kind: "asking" | "done" }, ask: Ask | undefined, lastReply?: string): Preview {
  if (seat.kind === "asking") {
    return { identity: seat.identity, kind: "asking", heading: "Asking you", detail: oneLine(ask?.question ?? "") || null };
  }
  return { identity: seat.identity, kind: "done", heading: "Done, your turn", detail: oneLine(lastReply ?? "") || null };
}
