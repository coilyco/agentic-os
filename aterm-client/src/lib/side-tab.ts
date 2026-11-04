// Which tab the side panel opens on, by role, and the choice Kai made over it.
// The role table is the only place a role name decides a tab.

export const SIDE_TABS = ["messages", "views", "browser", "terminal"] as const;
export type SideTab = (typeof SIDE_TABS)[number];

/** First match wins. An exact slug is a role, a trailing dash is a family of roles. */
const ROLE_DEFAULTS: readonly { match: string; tab: SideTab }[] = [
  { match: "sysadmin-", tab: "terminal" },
  { match: "frontend-", tab: "browser" },
  { match: "prod-director", tab: "messages" },
];

const FALLBACK: SideTab = "messages";

/** What a seat has to show right now, for the tabs that can be empty. */
export interface TabFacts {
  /** The host streams a browser and this seat has a page open in it. */
  browser: boolean;
}

/** A tab that would open on an empty state is never the default, whatever the role. */
export function defaultSideTab(role: string, facts: TabFacts = { browser: true }): SideTab {
  const rule = ROLE_DEFAULTS.find(({ match }) => (match.endsWith("-") ? role.startsWith(match) : role === match));
  const tab = rule?.tab ?? FALLBACK;
  return tab === "browser" && !facts.browser ? FALLBACK : tab;
}

export function isSideTab(value: unknown): value is SideTab {
  return typeof value === "string" && (SIDE_TABS as readonly string[]).includes(value);
}

/** What Kai picked, by role, since the instances of a role are peers. */
export type SideTabOverrides = Record<string, SideTab>;

/** A pick of Kai's stands even when the tab is empty, since she chose it. */
export function sideTabFor(role: string, overrides: SideTabOverrides, facts?: TabFacts): SideTab {
  return overrides[role] ?? defaultSideTab(role, facts);
}

const KEY = "aterm.side-tab.v1";

export function parseOverrides(raw: string | null): SideTabOverrides {
  try {
    const parsed: unknown = JSON.parse(raw ?? "{}");
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return {};
    return Object.fromEntries(Object.entries(parsed).filter((entry): entry is [string, SideTab] => isSideTab(entry[1])));
  } catch {
    return {};
  }
}

export function loadOverrides(storage: Pick<Storage, "getItem"> | undefined = safeStorage()): SideTabOverrides {
  try {
    return parseOverrides(storage?.getItem(KEY) ?? null);
  } catch {
    return {};
  }
}

/** Remembers a pick. With storage off it lasts until reload. */
export function storeOverride(role: string, tab: SideTab, storage: Pick<Storage, "getItem" | "setItem"> | undefined = safeStorage()): void {
  try {
    storage?.setItem(KEY, JSON.stringify({ ...loadOverrides(storage), [role]: tab }));
  } catch {
    // Nothing to do: the tab is already showing.
  }
}

function safeStorage(): Storage | undefined {
  try {
    return typeof localStorage === "undefined" ? undefined : localStorage;
  } catch {
    return undefined;
  }
}
