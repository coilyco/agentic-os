import { beforeEach, describe, expect, it, vi } from "vitest";
import { clearSends, describeImages, IMAGE_SENDS, jumpTo, KEEP_MS, MAX_CHARS, MAX_SEATS, newer, NOT_WALKING, older, onFirstLine, onLastLine, parseHistory, PER_SEAT, recentSends, recordSend, resetHistoryMemory, withSend, type Entry } from "./send-history";

const sends = (...texts: string[]): Entry[] => texts.map((text, at) => ({ text, at }));

describe("the walk", () => {
  // Newest first, as the list is held.
  const list = sends("third", "second", "first");

  it("goes up, up, down, down, and the second down is the draft again", () => {
    const one = older(list, NOT_WALKING, "half typed")!;
    expect(one.text).toBe("third");
    const two = older(list, one.walk, "third")!;
    expect(two.text).toBe("second");
    const back = newer(list, two.walk)!;
    expect(back.text).toBe("third");
    const draft = newer(list, back.walk)!;
    expect(draft.text).toBe("half typed");
    expect(draft.walk).toEqual(NOT_WALKING);
  });

  it("keeps the draft from the first step, not a recalled send", () => {
    const one = older(list, NOT_WALKING, "mine")!;
    const two = older(list, one.walk, "third")!;
    expect(two.walk.draft).toBe("mine");
  });

  it("stops at the oldest, and down does nothing when not walking", () => {
    let walk = NOT_WALKING;
    for (let i = 0; i < 3; i++) walk = older(list, walk, "")!.walk;
    expect(older(list, walk, "")).toBeNull();
    expect(newer(list, NOT_WALKING)).toBeNull();
  });

  it("has nothing to recall for a seat with no sends", () => {
    expect(older([], NOT_WALKING, "x")).toBeNull();
  });

  it("lets a tap jump to any send and still return to the draft", () => {
    const hit = jumpTo(list, NOT_WALKING, "draft words", 2)!;
    expect(hit.text).toBe("first");
    expect(newer(list, jumpTo(list, hit.walk, "first", 0)!.walk)!.text).toBe("draft words");
    expect(jumpTo(list, NOT_WALKING, "", 9)).toBeNull();
  });
});

describe("the first-line rule", () => {
  it("recalls from the first line and not from a later one", () => {
    expect(onFirstLine("", 0, false)).toBe(true);
    expect(onFirstLine("one line", 4, false)).toBe(true);
    expect(onFirstLine("one\ntwo", 7, false)).toBe(false);
    expect(onFirstLine("one\ntwo", 2, false)).toBe(true);
  });

  it("moves the caret first on a wrapped line, then recalls from its start", () => {
    expect(onFirstLine("a long wrapped line", 8, true)).toBe(false);
    expect(onFirstLine("a long wrapped line", 0, true)).toBe(true);
  });

  it("walks forward only from the last line", () => {
    expect(onLastLine("one\ntwo", 1, false)).toBe(false);
    expect(onLastLine("one\ntwo", 7, false)).toBe(true);
    expect(onLastLine("wrapped", 3, true)).toBe(false);
    expect(onLastLine("wrapped", 7, true)).toBe(true);
  });
});

describe("what is kept", () => {
  const NOW = 1_000_000_000_000;

  it("records a send once when it is repeated, moved to now", () => {
    const first = withSend({}, "mac/imp", "yes", NOW);
    const again = withSend(first, "mac/imp", "yes", NOW + 5);
    expect(again["mac/imp"]).toEqual([{ text: "yes", at: NOW + 5 }]);
    expect(withSend(again, "mac/imp", "no", NOW + 6)["mac/imp"]).toHaveLength(2);
  });

  it("is per host and seat", () => {
    const stored = withSend(withSend({}, "mac/imp", "a", NOW), "phone/imp", "b", NOW);
    expect(stored["mac/imp"]?.map((one) => one.text)).toEqual(["a"]);
    expect(stored["phone/imp"]?.map((one) => one.text)).toEqual(["b"]);
  });

  it("keeps the newest sends of a seat and the newest seats", () => {
    let stored = {};
    for (let i = 0; i < PER_SEAT + 5; i++) stored = withSend(stored, "mac/imp", `send ${i}`, NOW + i);
    expect((stored as Record<string, Entry[]>)["mac/imp"]).toHaveLength(PER_SEAT);
    expect((stored as Record<string, Entry[]>)["mac/imp"]?.[0]?.text).toBe("send 5");
    for (let i = 0; i < MAX_SEATS + 3; i++) stored = withSend(stored, `mac/seat-${i}`, "x", NOW + 1000 + i);
    expect(Object.keys(stored)).toHaveLength(MAX_SEATS);
  });

  it("ignores a blank send and one too long to store", () => {
    expect(withSend({}, "k", "   ", NOW)).toEqual({});
    expect(withSend({}, "k", "x".repeat(MAX_CHARS + 1), NOW)).toEqual({});
  });

  it("drops what is older than the keep time and what is malformed", () => {
    const raw = JSON.stringify({ "mac/imp": [{ text: "old", at: NOW - KEEP_MS - 1 }, { text: "new", at: NOW }, { text: 5, at: NOW }, "junk"], bad: "nope" });
    expect(parseHistory(raw, NOW)["mac/imp"]?.map((one) => one.text)).toEqual(["new"]);
    expect(parseHistory("{not json", NOW)).toEqual({});
    expect(parseHistory(null, NOW)).toEqual({});
  });
});

describe("image sends", () => {
  const NOW = 1_000_000_000_000;
  const shot = (name: string, size = 100) => ({ name, size });
  const attached = (id: string, ...files: { name: string; size: number }[]) => ({ ...describeImages(files), id });

  it("keeps an image send with no words, which a words-only send would drop", () => {
    expect(withSend({}, "k", "", NOW)).toEqual({});
    const stored = withSend({}, "k", "", NOW, attached("a", shot("one.png")));
    expect(stored["k"]).toEqual([{ text: "", at: NOW, images: 1, id: "a", sig: "one.png:100" }]);
  });

  it("makes a press repeated while Send is held one entry, with the first id", () => {
    const first = withSend({}, "k", "look", NOW, attached("a", shot("one.png"), shot("two.png")));
    const again = withSend(first, "k", "look", NOW + 5, attached("b", shot("one.png"), shot("two.png")));
    expect(again["k"]).toHaveLength(1);
    expect(again["k"]?.[0]).toMatchObject({ id: "a", images: 2, at: NOW + 5 });
  });

  it("treats the same words with other images as another send", () => {
    const first = withSend({}, "k", "look", NOW, attached("a", shot("one.png")));
    expect(withSend(first, "k", "look", NOW + 5, attached("b", shot("two.png")))["k"]).toHaveLength(2);
    expect(withSend(first, "k", "look", NOW + 5)["k"]).toHaveLength(2);
  });

  it("keeps images for the newest ten image sends only, and an older one keeps its words", () => {
    let stored = {};
    for (let i = 0; i < IMAGE_SENDS + 2; i++) stored = withSend(stored, "k", i % 2 ? `note ${i}` : "", NOW + i, attached(`id${i}`, shot(`s${i}.png`)));
    const list = (stored as Record<string, Entry[]>)["k"]!;
    expect(list.filter((one) => one.images)).toHaveLength(IMAGE_SENDS);
    expect(list.some((one) => one.text === "" && !one.images), "a wordless send lost its images and stayed").toBe(false);
    expect(list.find((one) => one.text === "note 1"), "a send with words keeps the words").toEqual({ text: "note 1", at: NOW + 1 });
  });

  it("reads an entry with images and no words back from storage", () => {
    const raw = JSON.stringify({ k: [{ text: "", at: NOW, images: 2, id: "a", sig: "x" }, { text: "", at: NOW }] });
    expect(parseHistory(raw, NOW)["k"]).toHaveLength(1);
  });
});

describe("on the device", () => {
  const store = new Map<string, string>();
  beforeEach(() => {
    store.clear();
    resetHistoryMemory();
    vi.stubGlobal("localStorage", { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v), removeItem: (k: string) => void store.delete(k) });
  });

  it("survives a restart, newest first", () => {
    recordSend("mac/imp", "one");
    recordSend("mac/imp", "two");
    resetHistoryMemory();
    expect(recentSends("mac/imp").map((one) => one.text)).toEqual(["two", "one"]);
  });

  it("hands back an id for new images and none for the same press again", () => {
    const file = new File(["x"], "shot.png", { type: "image/png" });
    const first = recordSend("mac/imp", "", [file]);
    expect(first.fresh).toBe(true);
    expect(first.id).toBeTruthy();
    const again = recordSend("mac/imp", "", [file]);
    expect(again).toEqual({ id: first.id, fresh: false });
    expect(recordSend("mac/imp", "words only")).toEqual({ fresh: false });
  });

  it("clears one seat and leaves the others", () => {
    recordSend("mac/imp", "one");
    recordSend("mac/other", "keep");
    clearSends("mac/imp");
    resetHistoryMemory();
    expect(recentSends("mac/imp")).toEqual([]);
    expect(recentSends("mac/other")).toHaveLength(1);
  });

  it("keeps sends in memory when storage throws", () => {
    vi.stubGlobal("localStorage", { getItem: () => { throw new Error("off"); }, setItem: () => { throw new Error("off"); }, removeItem: () => { throw new Error("off"); } });
    resetHistoryMemory();
    recordSend("mac/imp", "still here");
    expect(recentSends("mac/imp").map((one) => one.text)).toEqual(["still here"]);
  });
});
