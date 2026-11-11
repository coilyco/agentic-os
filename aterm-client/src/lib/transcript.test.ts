import { describe, expect, it } from "vitest";
import { decodeItem, decodeReply, mergeItems, refusalText, rowsFor, withoutPeers, type TranscriptItem } from "./transcript";

const turn: TranscriptItem[] = [
  { id: "1", kind: "prompt", text: "Why does the wrapped row hide the stripe?" },
  { id: "2", kind: "reply", text: "I will read the component first." },
  { id: "3", kind: "tool", tool: { name: "Read", class: "read", summary: "Read Terminal.svelte", output: "a\nb\nc\nd\ne", ok: true } },
  { id: "4", kind: "tool", tool: { name: "Bash", class: "shell", summary: "Ran just gate", output: "0 errors\n847 passed\n", ok: false, truncated: true } },
  { id: "5", kind: "reply", text: "The stripe was placed by row number.", complete: true },
  { id: "6", kind: "note", text: "Context compacted." },
];

describe("what each level shows", () => {
  it("shows Focus as the prompt, one line per tool and the final reply, with no result text", () => {
    const rows = rowsFor(turn, "focus");
    expect(rows.map((row) => row.key)).toEqual(["1", "3", "4", "5"]);
    expect(rows.filter((row) => row.kind === "tool").every((row) => row.kind === "tool" && row.lines.length === 0 && row.hidden === 0)).toBe(true);
  });

  it("keeps every reply and three lines of each result in Default, and says how many it cut", () => {
    const rows = rowsFor(turn, "default");
    expect(rows.map((row) => row.key)).toEqual(["1", "2", "3", "4", "5", "6"]);
    const read = rows.find((row) => row.key === "3");
    expect(read).toMatchObject({ kind: "tool", lines: ["a", "b", "c"], hidden: 2 });
  });

  it("keeps every line in Verbose, and what the daemon cut stays marked", () => {
    const rows = rowsFor(turn, "verbose");
    expect(rows.find((row) => row.key === "3")).toMatchObject({ lines: ["a", "b", "c", "d", "e"], hidden: 0 });
    expect(rows.find((row) => row.key === "4")).toMatchObject({ lines: ["0 errors", "847 passed"], ok: false, cut: true });
  });

  it("takes the last reply of each turn in Focus, so an earlier turn keeps its own", () => {
    const two: TranscriptItem[] = [
      ...turn,
      { id: "7", kind: "prompt", text: "And the links?" },
      { id: "8", kind: "reply", text: "Mapped from the text under the finger." },
    ];
    expect(rowsFor(two, "focus").map((row) => row.key)).toEqual(["1", "3", "4", "5", "7", "8"]);
  });

  it("shows nothing for an empty transcript", () => {
    expect(rowsFor([], "focus")).toEqual([]);
  });
});

describe("reading a reply off the wire", () => {
  it("drops an item with no id or an unknown kind, so one odd line never breaks the view", () => {
    expect(decodeItem({ kind: "reply", text: "x" })).toBeNull();
    expect(decodeItem({ id: "1", kind: "thinking" })).toBeNull();
    expect(decodeItem("nope")).toBeNull();
    expect(decodeItem({ id: "1", kind: "reply", text: "hi", at: "2026-11-10T21:00:00Z" })).toMatchObject({ id: "1", at: Date.parse("2026-11-10T21:00:00Z") });
  });

  it("reads a tool step's snake case fields and falls back to its name for a missing summary", () => {
    const item = decodeItem({ id: "9", kind: "tool", tool: { name: "Bash", output: "ok", output_bytes: 2, truncated: true } });
    expect(item).toMatchObject({ tool: { name: "Bash", class: "other", summary: "Bash", outputBytes: 2, truncated: true } });
  });

  it("reads a tool's class and an edit's counts, and an unknown class as other", () => {
    expect(decodeItem({ id: "1", kind: "tool", tool: { name: "Edit", class: "edit", summary: "Edited a.go +12 -3", diff: { files: 1, added: 12, removed: 3 } } })).toMatchObject({ tool: { class: "edit", diff: { files: 1, added: 12, removed: 3 } } });
    expect(decodeItem({ id: "1", kind: "tool", tool: { name: "X", class: "teleport" } })).toMatchObject({ tool: { class: "other" } });
    expect(decodeItem({ id: "1", kind: "tool", tool: { name: "X", diff: { files: 1 } } })?.tool?.diff).toBeUndefined();
  });

  it("reads who a peer item came from", () => {
    expect(decodeItem({ id: "3", kind: "peer", text: "x", from: { role: "eng-platform", identity: "Beetle-Ox" } })).toMatchObject({ from: { role: "eng-platform", identity: "Beetle-Ox" } });
    expect(decodeItem({ id: "3", kind: "peer", text: "x", from: { role: "eng-platform" } })?.from).toBeUndefined();
  });

  it("decodes a reply and survives a missing items list", () => {
    expect(decodeReply("s", { reader: "claude", items: [{ id: "1", kind: "prompt", text: "hi" }, { bad: true }], cursor: "c1", older: "o1", more: true })).toMatchObject({ reader: "claude", cursor: "c1", older: "o1", more: true, items: [{ id: "1" }] });
    expect(decodeReply("s", { reader: "codex", items: [] }).older).toBeUndefined();
    expect(decodeReply("s", {}).items).toEqual([]);
  });

  it("replaces an item in place when its id comes back, and appends new ones", () => {
    const merged = mergeItems([{ id: "1", kind: "reply", text: "so far" }], [{ id: "1", kind: "reply", text: "whole", complete: true }, { id: "2", kind: "prompt", text: "next" }]);
    expect(merged).toEqual([{ id: "1", kind: "reply", text: "whole", complete: true }, { id: "2", kind: "prompt", text: "next" }]);
  });

  it("says every refusal in words, and a reason it does not know in a plain sentence", () => {
    for (const reason of ["no_transcript", "ambiguous", "unreadable_format", "unsupported_harness", "passkey_required", "bad_cursor"]) expect(refusalText(reason)).not.toMatch(/_/);
    expect(refusalText("something_new")).toBe("The host would not send this seat's transcript.");
    expect(refusalText(undefined)).toBe("The host would not send this seat's transcript.");
  });
});

describe("peer messages", () => {
  const peer: TranscriptItem[] = [
    { id: "1", kind: "prompt", text: "Run the gate." },
    { id: "2", kind: "reply", text: "Gate is green.", complete: true },
    { id: "3", kind: "peer", text: "[from eng-platform Beetle-Ox] Is the frame fixed?" },
    { id: "4", kind: "tool", tool: { name: "Bash", class: "shell", summary: "Ran git log" }, for: "3" },
    { id: "5", kind: "reply", text: "Yes, it is fixed.", complete: true, for: "3" },
    { id: "6", kind: "prompt", text: "Thanks." },
    { id: "7", kind: "reply", text: "Welcome.", complete: true },
  ];

  it("leaves a peer message out at every level, with the replies and steps tagged to it", () => {
    for (const level of ["focus", "default", "verbose"] as const) {
      expect(rowsFor(peer, level).map((row) => row.key), level).toEqual(["1", "2", "6", "7"]);
    }
  });

  it("never drops the person's own turns", () => {
    expect(withoutPeers(peer).filter((item) => item.kind === "prompt").map((item) => item.id)).toEqual(["1", "6"]);
  });

  it("reads an item with no `for` as answering nobody, whatever the harness, so it shows", () => {
    const untagged: TranscriptItem[] = [{ id: "1", kind: "prompt", text: "Go." }, { id: "2", kind: "peer", text: "[from x y] hi" }, { id: "3", kind: "reply", text: "Working on it." }];
    expect(withoutPeers(untagged).map((item) => item.id)).toEqual(["1", "3"]);
  });

  it("keeps an item the daemon calls shared and marks it, since it may be the person's own work", () => {
    const mid: TranscriptItem[] = [
      { id: "1", kind: "prompt", text: "Refactor it." },
      { id: "2", kind: "tool", tool: { name: "Edit", class: "edit", summary: "Edited a.ts +3 -1" } },
      { id: "3", kind: "peer", text: "[from eng-platform Beetle-Ox] ping" },
      { id: "4", kind: "reply", text: "Refactor done.", complete: true, for: "3", shared: true },
    ];
    const rows = rowsFor(mid, "focus");
    expect(rows.map((row) => row.key)).toEqual(["1", "2", "4"]);
    expect(rows.map((row) => row.peer?.id)).toEqual([undefined, undefined, "3"]);
    expect(rows[2]!.peer).toEqual({ id: "3", text: "[from eng-platform Beetle-Ox] ping" });
  });

  it("decodes the peer kind, the answer link and the shared mark", () => {
    expect(decodeItem({ id: "3", kind: "peer", text: "x" })).toMatchObject({ kind: "peer" });
    expect(decodeItem({ id: "4", kind: "reply", text: "y", for: "3", shared: true })).toMatchObject({ for: "3", shared: true });
    expect(decodeItem({ id: "4", kind: "reply", text: "y", shared: "yes" })?.shared).toBeUndefined();
  });
});

describe("text the host cut", () => {
  it("carries the cut flag to the row, so the view can say so", () => {
    const rows = rowsFor([{ id: "1", kind: "prompt", text: "a" }, { id: "2", kind: "reply", text: "b", truncated: true }], "verbose");
    expect(rows.map((row) => (row.kind === "tool" ? false : row.cut))).toEqual([false, true]);
    expect(decodeItem({ id: "2", kind: "reply", text: "b", truncated: true, text_bytes: 99 })).toMatchObject({ truncated: true });
  });
});
