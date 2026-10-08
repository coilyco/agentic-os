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

export function defaultSideTab(role: string): SideTab {
  const rule = ROLE_DEFAULTS.find(({ match }) => (match.endsWith("-") ? role.startsWith(match) : role === match));
  return rule?.tab ?? FALLBACK;
}

export function isSideTab(value: unknown): value is SideTab {
  return typeof value === "string" && (SIDE_TABS as readonly string[]).includes(value);
}

/** What Kai picked, by role, since the instances of a role are peers. */
export type SideTabOverrides = Record<string, SideTab>;

export function sideTabFor(role: string, overrides: SideTabOverrides): SideTab {
  return overrides[role] ?? defaultSideTab(role);
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
