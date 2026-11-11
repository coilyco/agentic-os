import { describe, expect, it } from "vitest";
import { KEEP_MS, MAX_ITEMS, MAX_SEATS, catchUp, markerRow, planPrune, trimForStore, type Snapshot } from "./transcript-cache";
import type { TranscriptItem } from "./transcript";

const item = (id: string, kind: TranscriptItem["kind"] = "reply"): TranscriptItem => ({ id, kind, text: `text ${id}` });
const held = (ids: string[], older?: string): Snapshot => ({ key: "h/s", at: 0, items: ids.map((id) => item(id)), ...(older ? { older } : {}) });

describe("what is kept", () => {
  it("is the newest items, and a tool keeps its summary but never its output", () => {
    const many = Array.from({ length: MAX_ITEMS + 5 }, (_, at) => item(`i${at}`));
    expect(trimForStore(many)).toHaveLength(MAX_ITEMS);
    expect(trimForStore(many)[0]!.id).toBe("i5");
    const tool: TranscriptItem = { id: "t", kind: "tool", tool: { name: "Bash", class: "shell", summary: "Ran ls", output: "secret=hunter2\n", outputBytes: 15, ok: true } };
    const [kept] = trimForStore([tool]);
    expect(kept!.tool).toEqual({ name: "Bash", class: "shell", summary: "Ran ls", outputBytes: 15, ok: true });
    expect(JSON.stringify(kept)).not.toContain("hunter2");
  });

  it("drops a day old row, then the oldest past the cap, and never the seat just saved", () => {
    const now = 10 * KEEP_MS;
    const rows = [{ key: "old", at: now - KEEP_MS - 1 }, ...Array.from({ length: MAX_SEATS + 2 }, (_, at) => ({ key: `s${at}`, at: now - 1000 + at }))];
    const drop = planPrune(rows, "s0", now);
    expect(drop).toContain("old");
    expect(drop).not.toContain("s0");
    expect(drop).toHaveLength(1 + 2);
    expect(planPrune([{ key: "lone", at: now - KEEP_MS - 1 }], "lone", now), "the saved seat stays however old").toEqual([]);
  });
});

describe("catching up", () => {
  it("marks the first item the person has not seen, and joins the pages without a gap", () => {
    const result = catchUp(held(["a", "b", "c"], "o1"), [item("b"), item("c"), item("d"), item("e")], "o9");
    expect(result.items.map((each) => each.id)).toEqual(["a", "b", "c", "d", "e"]);
    expect(result.newFrom).toBe("d");
    expect(result.older, "held reaches further back than the page read").toBe("o1");
    expect(result.gap).toBe(false);
  });

  it("marks nothing when nothing is new", () => {
    expect(catchUp(held(["a", "b"]), [item("a"), item("b")], undefined).newFrom).toBeUndefined();
  });

  it("brings a changed item back in place, which restores a tool's output", () => {
    const withOutput: TranscriptItem = { id: "t", kind: "tool", tool: { name: "Bash", class: "shell", summary: "Ran ls", output: "a\nb" } };
    const result = catchUp({ ...held(["a"]), items: [item("a"), { ...withOutput, tool: { ...withOutput.tool!, output: undefined } }] }, [withOutput], undefined);
    expect(result.items[1]!.tool!.output).toBe("a\nb");
  });

  it("drops the held page when the page read shares nothing with it, and says everything is new", () => {
    const result = catchUp(held(["a", "b"]), [item("x"), item("y")], "o7");
    expect(result.items.map((each) => each.id)).toEqual(["x", "y"]);
    expect(result).toMatchObject({ gap: true, newFrom: "x", older: "o7" });
  });

  it("reaches back with the page's own pointer when the page read starts later than the held one", () => {
    expect(catchUp(held(["a", "b"], "o1"), [item("c"), item("d")], "o2").older).toBe("o2");
  });
});

describe("where the marker goes", () => {
  const items = ["a", "b", "c", "d"].map((id) => item(id));
  it("is the first drawn row at or after the first new item", () => {
    expect(markerRow(items, ["a", "b", "c", "d"], "c")).toBe("c");
    expect(markerRow(items, ["a", "b", "d"], "c"), "c is not drawn at this view, so d takes it").toBe("d");
    expect(markerRow(items, ["a", "b"], "c"), "nothing new is drawn").toBeUndefined();
    expect(markerRow(items, ["a"], undefined)).toBeUndefined();
    expect(markerRow(items, ["a"], "zzz")).toBeUndefined();
  });
});
