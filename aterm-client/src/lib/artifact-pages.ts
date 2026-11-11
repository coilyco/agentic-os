// What the reader shows of a seat's page (COI-3330): its rows, and the sandbox its source
// is drawn in. The source is the seat's own file, so it runs with no origin of its own.
import { viewDocument } from "./mcp-apps";
import { isArtifactUrl } from "./open-external";
import type { SeatArtifact } from "./protocol";

/** The frame's `sandbox`. Never allow-same-origin: that would give a page our storage. */
export const PAGE_SANDBOX = "allow-scripts";

/** The hosts the Artifact tool lets a page load from, so it behaves as written. */
export const PAGE_HOSTS = [
  "https://cdnjs.cloudflare.com",
  "https://cdn.jsdelivr.net",
  "https://unpkg.com",
  "https://cdn.tailwindcss.com",
  "https://code.jquery.com",
  "https://fonts.googleapis.com",
  "https://fonts.gstatic.com",
];

/** The page's source with its policy pinned first: those hosts and nothing else. */
export function pageDocument(html: string): string {
  return viewDocument(html, { resourceDomains: PAGE_HOSTS });
}

/** A page that calls window.claude reads and saves state a sandbox here does not have. */
export function readsLiveState(html: string): boolean {
  return /window\s*\.\s*claude\b/.test(html);
}

export interface PageRow {
  id: string;
  title: string;
  /** Who posted it, by name. */
  from: string;
  waiting: boolean;
  seatEnded: boolean;
  /** Its own state is on claude.ai, so the reader leads with the open button. */
  answerOnPage: boolean;
  /** Only an address that is a claude.ai artifact is offered. */
  openable: boolean;
}

interface Seat {
  id: string;
  identity: string;
}

/** Waiting pages first, then the rest in the order they arrived. */
export function pageRows(pages: readonly SeatArtifact[], sessions: readonly Seat[]): PageRow[] {
  const names = new Map(sessions.map((seat) => [seat.id, seat.identity]));
  const rows = pages.map((page, index) => ({
    index,
    row: {
      id: page.id,
      title: page.title || "A page",
      from: names.get(page.session) ?? page.session,
      waiting: page.waiting && !page.seatEnded,
      seatEnded: page.seatEnded,
      answerOnPage: readsLiveState(page.html),
      openable: isArtifactUrl(page.url),
    } satisfies PageRow,
  }));
  return rows.sort((a, b) => Number(b.row.waiting) - Number(a.row.waiting) || a.index - b.index).map(({ row }) => row);
}

/** The line under a title: who posted it, and when that seat is gone, that it is. */
export function fromText(row: Pick<PageRow, "from" | "seatEnded">): string {
  return row.seatEnded ? `from ${row.from}, seat ended` : `from ${row.from}`;
}
