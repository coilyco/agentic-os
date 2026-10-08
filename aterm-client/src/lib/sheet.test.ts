import { describe, expect, it } from "vitest";
import { escapeHides, openAfterPick } from "./sheet";

describe("openAfterPick", () => {
  it("raises a put-away sheet for any tab", () => {
    expect(openAfterPick(true, false, "messages", "messages")).toBe(true);
    expect(openAfterPick(true, false, "messages", "terminal")).toBe(true);
  });

  it("puts the sheet away when the tab already showing is picked again", () => {
    expect(openAfterPick(true, true, "views", "views")).toBe(false);
  });

  it("keeps the sheet raised when another tab is picked", () => {
    expect(openAfterPick(true, true, "views", "browser")).toBe(true);
  });

  it("leaves the state alone beside the terminal, where there is no sheet", () => {
    expect(openAfterPick(false, false, "messages", "views")).toBe(false);
    expect(openAfterPick(false, true, "views", "views")).toBe(true);
  });
});

describe("escapeHides", () => {
  it("answers on the tab strip whatever tab is showing", () => {
    expect(escapeHides(true, true, "terminal", true)).toBe(true);
    expect(escapeHides(true, true, "browser", true)).toBe(true);
  });

  it("answers in the reading tabs", () => {
    expect(escapeHides(true, true, "messages", false)).toBe(true);
    expect(escapeHides(true, true, "views", false)).toBe(true);
  });

  it("leaves Escape to the shell and the remote page", () => {
    expect(escapeHides(true, true, "terminal", false)).toBe(false);
    expect(escapeHides(true, true, "browser", false)).toBe(false);
  });

  it("does nothing when there is no raised sheet", () => {
    expect(escapeHides(true, false, "messages", true)).toBe(false);
    expect(escapeHides(false, true, "messages", true)).toBe(false);
  });
});
