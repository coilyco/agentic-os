import { describe, expect, it } from "vitest";
import { cardMemo, forgetCardMemos, memoKey } from "./card-memo.svelte";

const options = (...labels: string[]) => labels.map((label) => ({ label, description: "", freeText: false }));

describe("cardMemo", () => {
  it("hands back the same state for the same seat and options, so a rebuilt card keeps its draft", () => {
    const key = memoKey("seat-a", options("Yes", "No"));
    const first = cardMemo(key, []);
    first.text = "half a thought";
    first.writing = 2;
    expect(cardMemo(key, [])).toMatchObject({ text: "half a thought", writing: 2 });
    forgetCardMemos("seat-a");
  });

  it("keeps seats and different questions apart", () => {
    const a = cardMemo(memoKey("seat-a", options("Yes", "No")), []);
    a.text = "for a";
    expect(cardMemo(memoKey("seat-b", options("Yes", "No")), []).text).toBe("");
    expect(cardMemo(memoKey("seat-a", options("Red", "Blue")), []).text).toBe("");
    forgetCardMemos("seat-a");
    forgetCardMemos("seat-b");
  });

  it("starts clean once forgotten, so an answered card's draft never reappears", () => {
    const key = memoKey("seat-c", options("Yes", "No"));
    cardMemo(key, []).text = "old";
    forgetCardMemos("seat-c");
    expect(cardMemo(key, []).text).toBe("");
    forgetCardMemos("seat-c");
  });

  it("starts with the rows already ticked on screen", () => {
    expect(cardMemo(memoKey("seat-d", options("A", "B")), [1]).picked).toEqual([1]);
    forgetCardMemos("seat-d");
  });
});
