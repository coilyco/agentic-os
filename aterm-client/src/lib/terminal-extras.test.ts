import { describe, expect, it } from "vitest";
import { findCountText, linkTarget, opensLink, wantsFind } from "./terminal-extras";

const key = (init: Partial<KeyboardEventInit> & { key: string }) => ({ metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, ...init });

describe("wantsFind", () => {
  it("answers Cmd+F, and Ctrl+Shift+F where there is no Cmd", () => {
    expect(wantsFind(key({ key: "f", metaKey: true }))).toBe(true);
    expect(wantsFind(key({ key: "F", ctrlKey: true, shiftKey: true }))).toBe(true);
  });

  it("leaves plain Ctrl+F to the shell, where it moves the cursor forward", () => {
    expect(wantsFind(key({ key: "f", ctrlKey: true }))).toBe(false);
    expect(wantsFind(key({ key: "f" }))).toBe(false);
  });

  it("leaves other chords and other letters alone", () => {
    expect(wantsFind(key({ key: "g", metaKey: true }))).toBe(false);
    expect(wantsFind(key({ key: "f", metaKey: true, altKey: true }))).toBe(false);
  });
});

describe("linkTarget", () => {
  it("opens plain web addresses", () => {
    expect(linkTarget("https://github.com/coilyco/agentic-os/pull/197")).toBe("https://github.com/coilyco/agentic-os/pull/197");
    expect(linkTarget("http://localhost:5173/")).toBe("http://localhost:5173/");
  });

  it("refuses anything else a terminal might print", () => {
    expect(linkTarget("javascript:alert(1)")).toBeNull();
    expect(linkTarget("file:///etc/passwd")).toBeNull();
    expect(linkTarget("not a link")).toBeNull();
  });
});

describe("opensLink", () => {
  it("needs Cmd or Ctrl with a mouse, so a drag to select text is never a click on a link", () => {
    expect(opensLink({ metaKey: true, ctrlKey: false }, false)).toBe(true);
    expect(opensLink({ metaKey: false, ctrlKey: true }, false)).toBe(true);
    expect(opensLink({ metaKey: false, ctrlKey: false }, false)).toBe(false);
  });

  it("takes a plain tap on a touch screen, which has no modifier to hold", () => {
    expect(opensLink({ metaKey: false, ctrlKey: false }, true)).toBe(true);
  });
});

describe("findCountText", () => {
  it("says nothing before anything is typed", () => {
    expect(findCountText("", { resultIndex: -1, resultCount: 0 })).toBe("");
  });

  it("says when nothing matches", () => {
    expect(findCountText("zzz", { resultIndex: -1, resultCount: 0 })).toBe("No matches");
  });

  it("says which match of how many", () => {
    expect(findCountText("sweep", { resultIndex: 2, resultCount: 14 })).toBe("3 of 14");
  });

  it("says a count with a plus when there are too many to number", () => {
    expect(findCountText("e", { resultIndex: -1, resultCount: 1000 })).toBe("1000+");
  });
});
