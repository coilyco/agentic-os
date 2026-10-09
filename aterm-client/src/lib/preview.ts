// What hovering a glowing seat in the sidebar says, without opening it.

import type { Ask } from "./protocol";

export interface Preview {
  identity: string;
  kind: "asking" | "done";
  /** What the seat is doing to you, in the sidebar's own words. */
  heading: string;
  /** The question it asks. A finished seat has none: no reply text is held here. */
  detail: string | null;
}

const DETAIL_CHARS = 280;

export function previewFor(seat: { identity: string; kind: "asking" | "done" }, ask: Ask | undefined): Preview {
  if (seat.kind === "asking") {
    const question = ask?.question.trim().replace(/\s+/g, " ") ?? "";
    const detail = question.length > DETAIL_CHARS ? `${question.slice(0, DETAIL_CHARS).trimEnd()}…` : question;
    return { identity: seat.identity, kind: "asking", heading: "Asking you", detail: detail || null };
  }
  return { identity: seat.identity, kind: "done", heading: "Done, your turn", detail: null };
}
