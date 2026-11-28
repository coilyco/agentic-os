// The side panel as a bottom sheet, below the split breakpoint.
import type { SideTab } from "./side-tab";

/** Picking the tab already showing puts a raised sheet away. */
export function openAfterPick(narrow: boolean, open: boolean, current: SideTab, picked: SideTab): boolean {
  return narrow ? !(open && current === picked) : open;
}

// The shell and remote page take Escape, so only the strip and reading tabs answer it.
export function escapeAnswered(tab: SideTab, inTabStrip: boolean): boolean {
  return inTabStrip || tab === "messages" || tab === "views";
}

export function escapeHides(narrow: boolean, open: boolean, tab: SideTab, inTabStrip: boolean): boolean {
  return narrow && open && escapeAnswered(tab, inTabStrip);
}
