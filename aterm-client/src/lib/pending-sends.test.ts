import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cancelPending, notePending, pendingFor, sameWords, settlePending, SKEW_MS, UNSENT_AFTER_MS } from "./pending-sends.svelte";
import type { TranscriptItem } from "./transcript";

const prompt = (id: string, text: string, at?: number): TranscriptItem => ({ id, kind: "prompt", text, ...(at === undefined ? {} : { at }) });

describe("pending sends", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("shows at once and resolves to the one real entry, leaving none beside it", () => {
    notePending("a", "run the gate", Date.now());
    expect(pendingFor("a").map((one) => one.state)).toEqual(["pending"]);
    expect(settlePending("a", [prompt("p1", "run the gate")], true)).toBe(1);
    expect(pendingFor("a")).toHaveLength(0);
  });

  it("keeps an entry for words that are not in the transcript, and calls it unsent after the timeout", () => {
    notePending("b", "still here");
    settlePending("b", [prompt("p1", "something else")], true);
    expect(pendingFor("b")[0]?.state).toBe("pending");
    vi.advanceTimersByTime(UNSENT_AFTER_MS - 1);
    expect(pendingFor("b")[0]?.state).toBe("pending");
    vi.advanceTimersByTime(1);
    expect(pendingFor("b")[0]?.state).toBe("unsent");
  });

  it("resolves an unsent entry when the prompt arrives late", () => {
    notePending("c", "late one");
    vi.advanceTimersByTime(UNSENT_AFTER_MS + 1000);
    expect(settlePending("c", [prompt("p9", "late one")], true)).toBe(1);
    expect(pendingFor("c")).toHaveLength(0);
  });

  it("lets one prompt resolve one send, so two sends of the same words stay two", () => {
    notePending("d", "yes");
    notePending("d", "yes");
    expect(settlePending("d", [prompt("p1", "yes")], true)).toBe(1);
    expect(pendingFor("d")).toHaveLength(1);
    // The same prompt seen again does not resolve the second.
    expect(settlePending("d", [prompt("p1", "yes")], true)).toBe(0);
    expect(settlePending("d", [prompt("p2", "yes")], true)).toBe(1);
  });

  it("trusts a first read's prompt only by its stamp, so an old identical prompt is not this send", () => {
    const sentAt = Date.now();
    notePending("e", "ok", sentAt);
    expect(settlePending("e", [prompt("old", "ok", sentAt - SKEW_MS - 1)], false)).toBe(0);
    expect(settlePending("e", [prompt("noat", "ok")], false)).toBe(0);
    expect(settlePending("e", [prompt("new", "ok", sentAt - 1000)], false)).toBe(1);
  });

  it("is per seat", () => {
    notePending("f", "here");
    expect(settlePending("g", [prompt("p1", "here")], true)).toBe(0);
    expect(pendingFor("f")).toHaveLength(1);
  });

  it("cancels a refused send, and its timer goes with it", () => {
    const id = notePending("h", "refused");
    cancelPending(id);
    expect(pendingFor("h")).toHaveLength(0);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("reads an image's path before the words as the same words, and a folded line break too", () => {
    expect(sameWords("what is this", "[Attached image: /tmp/aterm/images/x.png] what is this")).toBe(true);
    expect(sameWords("line one\nline two", "line one line two")).toBe(true);
    expect(sameWords("what", "what is this")).toBe(false);
    expect(sameWords("", "anything")).toBe(false);
  });
});
