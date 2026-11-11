import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { POLL_MS, STOP_GRACE_MS, anchorAt, dictationAvailable, errorWords, parseAvailable, parseHeard, placeSpeech, refusalWords, startDictation } from "./dictation";

describe("what the plugin returns", () => {
  it("reads a recognizer's state, and an unreadable answer as idle with no words", () => {
    expect(parseHeard({ phase: "listening", text: "hello wor", error: null, seq: 3 })).toEqual({ phase: "listening", text: "hello wor", error: null, seq: 3 });
    expect(parseHeard({ phase: "failed", text: "", error: 7, seq: 4 }).error).toBe(7);
    expect(parseHeard(null)).toEqual({ phase: "idle", text: "", error: null, seq: 0 });
    expect(parseHeard({ phase: "from_the_future" }).phase).toBe("idle");
  });

  it("shows the mic only with a recognizer and a microphone that is not refused for good", () => {
    expect(parseAvailable({ available: true, permission: "granted" })).toBe(true);
    expect(parseAvailable({ available: true, permission: "prompt" })).toBe(true);
    expect(parseAvailable({ available: true, permission: "denied" })).toBe(false);
    expect(parseAvailable({ available: false, permission: "granted" })).toBe(false);
    expect(parseAvailable("garbage")).toBe(false);
  });

  it("is no mic in a browser and in an app built before the plugin", async () => {
    expect(await dictationAvailable(null)).toBe(false);
    expect(await dictationAvailable(async () => { throw new Error("no plugin"); })).toBe(false);
    expect(await dictationAvailable(async () => ({ available: true, permission: "prompt" }))).toBe(true);
  });
});

describe("what a stopped session says", () => {
  it("keeps words heard before a quiet end, and says nothing about it", () => {
    expect(errorWords(7, "hello there")).toBeNull();
    expect(errorWords(6, "hello there")).toBeNull();
  });

  it("says each known failure in words and an unknown one with its code", () => {
    expect(errorWords(7, "")).toContain("No words recognised");
    expect(errorWords(9, "")).toContain("Allow it in Android's app settings");
    expect(errorWords(2, "words")).toContain("network");
    expect(errorWords(99, "")).toContain("Android error 99");
    expect(errorWords(null, "")).toContain("without an answer");
  });

  it("turns a refused start into words by its code", () => {
    expect(refusalWords({ code: "denied", message: "x" })).toContain("not allowed");
    expect(refusalWords({ code: "unavailable", message: "x" })).toContain("no speech recognition service");
    expect(refusalWords({ code: "other", message: "boom" })).toBe("boom");
    expect(refusalWords(undefined)).toBe("Dictation would not start.");
  });
});

describe("where speech goes in the draft", () => {
  it("lands at the caret, spaced from words on either side", () => {
    const anchor = anchorAt("fix the bug", 7, 7);
    expect(placeSpeech(anchor, "login")).toEqual({ text: "fix the login bug", caret: 13 });
  });

  it("adds no space at the start, after a space or before a newline", () => {
    expect(placeSpeech(anchorAt("", 0, 0), "hello")).toEqual({ text: "hello", caret: 5 });
    expect(placeSpeech(anchorAt("hi ", 3, 3), "there")).toEqual({ text: "hi there", caret: 8 });
    expect(placeSpeech(anchorAt("hi\nnext", 2, 2), "there")).toEqual({ text: "hi there\nnext", caret: 8 });
  });

  it("replaces a selection, and empty speech leaves the draft as it was", () => {
    const anchor = anchorAt("say the wrong thing", 8, 13);
    expect(placeSpeech(anchor, "right")).toEqual({ text: "say the right thing", caret: 13 });
    expect(placeSpeech(anchorAt("keep", 4, 4), "   ")).toEqual({ text: "keep", caret: 4 });
  });

  it("clamps a caret past the end, and a reversed selection", () => {
    expect(anchorAt("abc", 9, 9)).toEqual({ before: "abc", after: "" });
    expect(anchorAt("abcdef", 4, 1)).toEqual({ before: "a", after: "ef" });
  });
});

describe("a session", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  /** A plugin that answers `poll` from a script and records every call. */
  function plugin(script: object[], reject?: { code: string; message: string }) {
    const calls: string[] = [];
    let at = 0;
    const invoke = async (command: string): Promise<unknown> => {
      calls.push(command.replace("plugin:dictation|", ""));
      if (command.endsWith("|start") && reject) throw reject;
      if (command.endsWith("|poll")) return script[Math.min(at++, script.length - 1)];
      return undefined;
    };
    return { invoke, calls };
  }

  const heard = (phase: string, text: string, seq: number, error: number | null = null) => ({ phase, text, error, seq });

  it("passes the words as they grow, and the final words once, then ends clean", async () => {
    const { invoke } = plugin([heard("listening", "", 1), heard("listening", "hello", 2), heard("listening", "hello", 2), heard("listening", "hello world", 3), heard("done", "hello world", 4)]);
    const text: [string, boolean][] = [];
    const ends: (string | null)[] = [];
    await startDictation({ onText: (words, final) => text.push([words, final]), onEnd: (note) => ends.push(note) }, invoke);
    await vi.advanceTimersByTimeAsync(POLL_MS * 8);
    // The unchanged answer at seq 2 is not repeated.
    expect(text).toEqual([["hello", false], ["hello world", false], ["hello world", true]]);
    expect(ends).toEqual([null]);
  });

  it("says why when the recognizer fails with nothing heard, and stops polling", async () => {
    const { invoke, calls } = plugin([heard("failed", "", 1, 7)]);
    const ends: (string | null)[] = [];
    await startDictation({ onText: () => {}, onEnd: (note) => ends.push(note) }, invoke);
    await vi.advanceTimersByTimeAsync(POLL_MS * 6);
    expect(ends).toEqual([errorWords(7, "")]);
    const polls = calls.filter((call) => call === "poll").length;
    await vi.advanceTimersByTimeAsync(POLL_MS * 6);
    expect(calls.filter((call) => call === "poll").length, "no poll after the session ended").toBe(polls);
  });

  it("keeps words heard before a failure, ending clean", async () => {
    const { invoke } = plugin([heard("listening", "half a sentence", 1), heard("failed", "", 2, 7)]);
    const text: [string, boolean][] = [];
    const ends: (string | null)[] = [];
    await startDictation({ onText: (words, final) => text.push([words, final]), onEnd: (note) => ends.push(note) }, invoke);
    await vi.advanceTimersByTimeAsync(POLL_MS * 4);
    expect(text).toEqual([["half a sentence", false]]);
    expect(ends).toEqual([null]);
  });

  it("refuses to start with words the person can act on", async () => {
    const { invoke } = plugin([], { code: "denied", message: "no mic" });
    await expect(startDictation({ onText: () => {}, onEnd: () => {} }, invoke)).rejects.toThrow("not allowed");
  });

  it("asks for the final words on stop, then waits for them", async () => {
    const { invoke, calls } = plugin([heard("listening", "so far", 1), heard("listening", "so far", 1), heard("done", "so far and more", 2)]);
    const text: [string, boolean][] = [];
    const session = await startDictation({ onText: (words, final) => text.push([words, final]), onEnd: () => {} }, invoke);
    await vi.advanceTimersByTimeAsync(POLL_MS);
    await session.stop();
    expect(calls).toContain("stop");
    await vi.advanceTimersByTimeAsync(POLL_MS * 4);
    expect(text.at(-1)).toEqual(["so far and more", true]);
  });

  it("gives up on a recognizer that never answers a stop, keeping what it heard", async () => {
    const { invoke, calls } = plugin([heard("listening", "kept words", 1)]);
    const text: [string, boolean][] = [];
    const ends: (string | null)[] = [];
    const session = await startDictation({ onText: (words, final) => text.push([words, final]), onEnd: (note) => ends.push(note) }, invoke);
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);
    await session.stop();
    await vi.advanceTimersByTimeAsync(STOP_GRACE_MS + POLL_MS * 3);
    expect(calls).toContain("cancel");
    expect(text.at(-1)).toEqual(["kept words", true]);
    expect(ends).toEqual([null]);
  });

  it("ends on cancel without a note and tells the plugin", async () => {
    const { invoke, calls } = plugin([heard("listening", "x", 1)]);
    const ends: (string | null)[] = [];
    const session = await startDictation({ onText: () => {}, onEnd: (note) => ends.push(note) }, invoke);
    await session.cancel();
    expect(ends).toEqual([null]);
    expect(calls).toContain("cancel");
  });
});
