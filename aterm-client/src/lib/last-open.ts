// The host this device last had open, so a reload returns to it. No seat (COI-3518).
const KEY = "aterm.last-open.v1";

export interface LastOpen {
  host: string;
}

export function parseLastOpen(text: string | null): LastOpen | null {
  try {
    const raw: unknown = JSON.parse(text ?? "null");
    if (typeof raw !== "object" || raw === null || typeof (raw as LastOpen).host !== "string") return null;
    return { host: (raw as LastOpen).host };
  } catch {
    return null;
  }
}

export function loadLastOpen(): LastOpen | null {
  try {
    return parseLastOpen(localStorage.getItem(KEY));
  } catch {
    return null;
  }
}

export function storeLastOpen(value: LastOpen): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(value));
  } catch {
    // Storage can be off (a private window). The next launch opens on the host list.
  }
}
