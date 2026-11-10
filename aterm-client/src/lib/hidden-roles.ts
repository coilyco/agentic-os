// Roles whose seats a person took out of the list (COI-3346). Per device, by role slug.
const KEY = "aterm.hidden-roles.v1";

export function parseHidden(text: string | null): string[] {
  try {
    const raw: unknown = JSON.parse(text ?? "null");
    return Array.isArray(raw) ? raw.filter((each): each is string => typeof each === "string") : [];
  } catch {
    return [];
  }
}

export function loadHidden(): string[] {
  try {
    return parseHidden(localStorage.getItem(KEY));
  } catch {
    return [];
  }
}

export function storeHidden(roles: readonly string[]): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(roles));
  } catch {
    // Storage can be off (a private window). The choice lasts until reload.
  }
}

/** The roles after a person flips one. Hiding a role twice is hiding it once. */
export function flipHidden(role: string, hidden: readonly string[]): string[] {
  return hidden.includes(role) ? hidden.filter((each) => each !== role) : [...hidden, role];
}

/** Hiding is for quiet seats. `keep` marks one that stays: asking, pinned, or open. */
export function splitHidden<T extends { role: string }>(sessions: readonly T[], hidden: readonly string[], keep: (session: T) => boolean): { shown: T[]; hidden: T[] } {
  const out = { shown: [] as T[], hidden: [] as T[] };
  for (const session of sessions) (hidden.includes(session.role) && !keep(session) ? out.hidden : out.shown).push(session);
  return out;
}
