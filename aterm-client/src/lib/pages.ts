// Pages seats asked to open (COI-3258): a card each, and nothing moves until a click.

export interface OpenedPage {
  id: number;
  sessionId: string;
  url: string;
  at: number;
}

/** Cards held at once. A seat that floods the screen with links loses the oldest. */
export const MAX_OFFERED = 5;

/** How long a click waits for the pane to hold control and load the page. */
export const SHOW_WAIT_MS = 30_000;

/** A seat offering an address it already offered is one card, not two. */
export function offer(list: readonly OpenedPage[], next: Omit<OpenedPage, "id">, id: number): OpenedPage[] {
  const rest = list.filter((page) => !(page.sessionId === next.sessionId && page.url === next.url));
  return [...rest, { ...next, id }].slice(-MAX_OFFERED);
}

/** The address a card shows: host and path, never the query, which may hold state. */
export function pageLabel(url: string, max = 56): string {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return "a page";
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return "a page";
  const text = `${parsed.host}${parsed.pathname === "/" ? "" : parsed.pathname}`;
  return `${text.length > max ? `${text.slice(0, max - 1)}…` : text}${parsed.search ? "?…" : ""}`;
}

/** The line a card leads with, and how many more wait behind it. */
export function cardText(identity: string, others: number): string {
  return others > 0 ? `${identity} opened a page, and ${others} more ${others === 1 ? "is" : "are"} waiting` : `${identity} opened a page`;
}
