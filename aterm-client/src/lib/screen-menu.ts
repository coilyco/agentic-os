// The native control for a harness menu the daemon's detector names (COI-3788): the
// resume picker and the /mcp server list. The daemon says which screen is up, this reads
// only the highlighted row, and the card drives the menu with its own keys.
import type { TuiState } from "./protocol";

export type MenuKind = "resume_picker" | "mcp_menu";

export interface ScreenMenu {
  kind: MenuKind;
  /** The highlighted row, as the menu prints it, or null when none is on screen. */
  selected: string | null;
  /** The row under it, which the resume picker uses for age, branch and directory. */
  detail: string | null;
}

export const MENU_KEYS = { up: "\x1b[A", down: "\x1b[B", open: "\r", close: "\x1b" } as const;

const CURSOR = /^\s*❯\s+(.*\S)\s*$/;
// The transcript above uses the same glyph. One server's page under /mcp is a plain
// choice, so the choice card reads it.
const HEADING: Record<MenuKind, RegExp> = { resume_picker: /^\s{0,4}Resume session\b/, mcp_menu: /^\s{0,4}MCP servers$/ };
const FOOTER: Record<MenuKind, RegExp> = { resume_picker: /Enter to resume\b/, mcp_menu: /Enter to view\b/ };

/** The menu the daemon reads, with its highlighted row, or null for any other screen. */
export function readScreenMenu(tui: TuiState | undefined, rows: readonly string[]): ScreenMenu | null {
  if (tui !== "resume_picker" && tui !== "mcp_menu") return null;
  const heading = rows.findLastIndex((row) => HEADING[tui].test(row));
  // With the heading scrolled away, no row is surely the menu's. The footer names it,
  if (heading === -1) {
    // wrapped mid-word when the terminal, not the harness, wrapped it.
    const last = rows.filter((row) => row.trim() !== "").slice(-4);
    return FOOTER[tui].test(last.join(" ")) || FOOTER[tui].test(last.join("")) ? { kind: tui, selected: null, detail: null } : null;
  }
  const at = rows.findLastIndex((row, index) => index > heading && CURSOR.test(row));
  if (at === -1) return { kind: tui, selected: null, detail: null };
  const selected = CURSOR.exec(rows[at]!)![1]!.replace(/\s{2,}/g, " - ");
  const below = rows[at + 1]?.trim() ?? "";
  return { kind: tui, selected, detail: tui === "resume_picker" && below !== "" ? below : null };
}

/** Whether the pane reads a choice card. Idle, busy, or a menu card says no. */
export function screenChoiceAllowed(tui: TuiState | undefined, menu: ScreenMenu | null): boolean {
  return tui !== "idle" && tui !== "busy" && menu === null;
}
