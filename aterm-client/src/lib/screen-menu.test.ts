import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { detectChoice } from "./choices";
import { readScreenMenu, screenChoiceAllowed } from "./screen-menu";

// The same Claude Code 2.1.332 screens the daemon's detector is tested on.
const recorded = (name: string): string[] =>
  readFileSync(new URL(`../../../aterm/testdata/claude-screens/${name}.txt`, import.meta.url), "utf8").replace(/\n+$/, "").split("\n");

describe("readScreenMenu", () => {
  it("reads the resume picker's highlighted conversation and the row under it", () => {
    expect(readScreenMenu("resume_picker", recorded("resume-picker"))).toEqual({
      kind: "resume_picker",
      selected: "eng-platform-wise-beetle",
      detail: "16 seconds ago · HEAD · 1.5MB · /tmp/aos/native/qj75/projects",
    });
  });

  it("names an empty resume picker with no row", () => {
    expect(readScreenMenu("resume_picker", recorded("resume-picker-empty"))).toEqual({ kind: "resume_picker", selected: null, detail: null });
  });

  it("reads the /mcp list's highlighted server, and leaves one server's page to the choice card", () => {
    expect(readScreenMenu("mcp_menu", recorded("mcp-menu"))?.selected).toBe("✔ local_coilyco_aterm - 14 tools");
    expect(readScreenMenu("mcp_menu", recorded("mcp-menu-connectors"))?.selected).toBe("◯ computer-use");
    expect(readScreenMenu("mcp_menu", recorded("mcp-server"))).toBeNull();
  });

  it("still names the menu when a short terminal scrolled its heading away, with no row it can vouch for", () => {
    const tail = recorded("resume-picker").slice(-8);
    expect(readScreenMenu("resume_picker", tail)).toEqual({ kind: "resume_picker", selected: null, detail: null });
    expect(readScreenMenu("mcp_menu", recorded("mcp-menu").slice(-6))).toEqual({ kind: "mcp_menu", selected: null, detail: null });
    expect(readScreenMenu("resume_picker", ["Ctrl+R to rename · Type to search · Enter t", "o resume · Esc to close"])?.kind).toBe("resume_picker");
  });

  it("draws nothing for a screen the daemon reads as anything else", () => {
    expect(readScreenMenu("idle", recorded("resume-picker"))).toBeNull();
    expect(readScreenMenu(undefined, recorded("mcp-menu"))).toBeNull();
  });
});

describe("screenChoiceAllowed", () => {
  it("leaves an old daemon and an unknown screen to the screen reader", () => {
    for (const tui of [undefined, "unknown", "choice", "permission", "login"] as const) expect(screenChoiceAllowed(tui, null), String(tui)).toBe(true);
  });

  it("draws no choice card while the daemon reads the seat as idle or busy", () => {
    expect(screenChoiceAllowed("idle", null)).toBe(false);
    expect(screenChoiceAllowed("busy", null)).toBe(false);
  });

  it("gives the /mcp list and the resume picker to the menu card, and one server's page to the choice card", () => {
    expect(screenChoiceAllowed("mcp_menu", readScreenMenu("mcp_menu", recorded("mcp-menu")))).toBe(false);
    expect(screenChoiceAllowed("resume_picker", readScreenMenu("resume_picker", recorded("resume-picker")))).toBe(false);
    expect(screenChoiceAllowed("mcp_menu", readScreenMenu("mcp_menu", recorded("mcp-server")))).toBe(true);
    expect(detectChoice(recorded("mcp-server"))?.options.map((option) => option.label)).toEqual(["View tools", "Clear authentication", "Reconnect", "Disable in this project", "Back"]);
  });

  it("agrees with the daemon that the recorded choice and permission screens are cards", () => {
    for (const name of ["q1", "q3", "review", "permission-create", "trust-folder"]) expect(detectChoice(recorded(name)), name).not.toBeNull();
  });
});
