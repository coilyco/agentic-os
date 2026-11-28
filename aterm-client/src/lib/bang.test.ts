import { describe, expect, it } from "vitest";
import { seatHint, startsWithBang, trackLine } from "./bang";

describe("startsWithBang", () => {
  it("reads the draft as a send trims it", () => {
    expect(startsWithBang("! ls")).toBe(true);
    expect(startsWithBang("  !ls")).toBe(true);
    expect(startsWithBang("ls !")).toBe(false);
    expect(startsWithBang("")).toBe(false);
  });
});

describe("seatHint", () => {
  it("names the seat's shell for a harness with a ! shell mode", () => {
    expect(seatHint("claude", "Breezy Griffin")).toBe("Runs in Breezy Griffin's shell. The seat sees the output.");
    expect(seatHint("codex", "Turtle-Ox")).toBe("Runs in Turtle-Ox's shell. The seat sees the output.");
  });

  it("says a message for a harness without one", () => {
    expect(seatHint("goose", "Turtle-Ox")).toBe("Sent to Turtle-Ox as a message. goose has no ! shell mode.");
    expect(seatHint("opencode", "Gem")).toContain("as a message");
  });

  it("claims neither for a harness nobody checked", () => {
    expect(seatHint("gemini", "Gem")).toBe("Sent to Gem. Whether gemini runs a ! line as a shell command is not checked.");
  });
});

describe("trackLine", () => {
  const typed = (...chunks: string[]) => chunks.reduce<string | null>((line, chunk) => trackLine(line, chunk), "");

  it("follows keys one at a time, with backspace", () => {
    expect(typed("!", " ", "l", "s")).toBe("! ls");
    expect(typed("!", "\x7f", "l")).toBe("l");
  });

  it("starts over on Enter, Ctrl-C and Ctrl-U", () => {
    expect(typed("! ls", "\r")).toBe("");
    expect(typed("! ls", "\x03")).toBe("");
    expect(typed("! ls", "\x15", "pwd")).toBe("pwd");
  });

  it("gives up on an arrow or Tab, until the next fresh line", () => {
    expect(typed("ls", "\x1b[A")).toBeNull();
    expect(typed("ls", "\x1b[A", "!")).toBeNull();
    expect(typed("l", "\t")).toBeNull();
    expect(typed("l", "\t", "\r", "!")).toBe("!");
  });

  it("reads a paste as text, and only its last line stays on the input", () => {
    expect(typed("\x1b[200~! ls\x1b[201~")).toBe("! ls");
    expect(typed("\x1b[200~echo a\n! ls\x1b[201~")).toBe("! ls");
  });
});
