import { describe, expect, it } from "vitest";
import { decodeSearch, rowsFor, type TranscriptItem } from "./transcript";
import { searchStatusText, stepMatch, wantsTranscriptFind } from "./transcript-search";

const chord = (init: { key: string; metaKey?: boolean; ctrlKey?: boolean; shiftKey?: boolean; altKey?: boolean }) => ({ metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, ...init });

describe("wantsTranscriptFind", () => {
  it("answers Cmd+F, Ctrl+F and Ctrl+Shift+F, since no shell is listening under a transcript", () => {
    expect(wantsTranscriptFind(chord({ key: "f", metaKey: true }))).toBe(true);
    expect(wantsTranscriptFind(chord({ key: "f", ctrlKey: true }))).toBe(true);
    expect(wantsTranscriptFind(chord({ key: "F", ctrlKey: true, shiftKey: true }))).toBe(true);
  });

  it("leaves plain F and Alt chords alone", () => {
    expect(wantsTranscriptFind(chord({ key: "f" }))).toBe(false);
    expect(wantsTranscriptFind(chord({ key: "f", ctrlKey: true, altKey: true }))).toBe(false);
  });
});

describe("stepMatch", () => {
  it("steps older and newer, and wraps at either end", () => {
    expect(stepMatch(2, 3, true)).toBe(1);
    expect(stepMatch(0, 3, true)).toBe(2);
    expect(stepMatch(2, 3, false)).toBe(0);
    expect(stepMatch(-1, 3, true)).toBe(2);
    expect(stepMatch(0, 0, true)).toBe(-1);
  });
});

describe("searchStatusText", () => {
  const base = { query: "gate", phase: "done" as const, at: 0, count: 0, total: 0, partial: false };

  it("counts from the newest match, as the search starts at the newest line", () => {
    expect(searchStatusText({ ...base, at: 11, count: 12, total: 12 })).toBe("1 of 12");
    expect(searchStatusText({ ...base, at: 0, count: 12, total: 12 })).toBe("12 of 12");
  });

  it("says none, a search under way, a failure, and a capped answer", () => {
    expect(searchStatusText(base)).toBe("No matches");
    expect(searchStatusText({ ...base, partial: true })).toBe("No matches in the part read");
    expect(searchStatusText({ ...base, phase: "searching" })).toBe("Searching");
    expect(searchStatusText({ ...base, phase: "failed" })).toBe("Search failed");
    expect(searchStatusText({ ...base, at: 499, count: 500, total: 700 })).toBe("1 of 700, newest 500 shown");
    expect(searchStatusText({ ...base, query: "  " })).toBe("");
  });
});

describe("decodeSearch", () => {
  it("keeps well formed matches and drops the rest", () => {
    const reply = decodeSearch({
      query: "gate",
      total: 3,
      more: true,
      matches: [{ id: "a", kind: "prompt", field: "text", snippet: "run the gate", at: "2026-11-27T08:00:00Z", before: "f:10" }, { id: "b", kind: "tool", field: "weird", snippet: "Ran just gate" }, { kind: "reply" }, "nonsense"],
    });
    expect(reply.matches).toEqual([
      { id: "a", kind: "prompt", field: "text", snippet: "run the gate", at: Date.parse("2026-11-27T08:00:00Z"), before: "f:10" },
      { id: "b", kind: "tool", field: "text", snippet: "Ran just gate" },
    ]);
    expect(reply.total).toBe(3);
    expect(reply.partial).toBe(true);
  });
});

describe("rowsFor with a match drawn whole", () => {
  const items: TranscriptItem[] = [
    { id: "p", kind: "prompt", text: "go" },
    { id: "r1", kind: "reply", text: "an early reply with the phrase" },
    { id: "t", kind: "tool", tool: { name: "Bash", class: "shell", summary: "Ran just test", output: "1\n2\n3\n4\nthe phrase\n" } },
    { id: "r2", kind: "reply", text: "done", complete: true },
  ];

  it("shows a reply Focus would leave out, and every line of a tool's output", () => {
    expect(rowsFor(items, "focus").map((row) => row.key)).toEqual(["p", "t", "r2"]);
    const rows = rowsFor(items, "focus", new Set(["r1", "t"]));
    expect(rows.map((row) => row.key)).toEqual(["p", "r1", "t", "r2"]);
    const tool = rows.find((row) => row.key === "t");
    expect(tool?.kind === "tool" && tool.lines).toEqual(["1", "2", "3", "4", "the phrase"]);
  });
});
