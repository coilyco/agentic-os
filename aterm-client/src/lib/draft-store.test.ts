import { describe, expect, it } from "vitest";
import { draftKey, KEEP_MS, MAX_CHARS, MAX_DRAFTS, parseDrafts, withDraft } from "./draft-store";

const NOW = 1_000_000_000;

describe("parseDrafts", () => {
  it("reads a stored draft", () => {
    expect(parseDrafts(JSON.stringify({ "h/s": { text: "hello", at: NOW } }), NOW)).toEqual({ "h/s": { text: "hello", at: NOW } });
  });

  it("reads nothing, junk or a wrong shape as no drafts", () => {
    expect(parseDrafts(null, NOW)).toEqual({});
    expect(parseDrafts("not json", NOW)).toEqual({});
    expect(parseDrafts(JSON.stringify({ a: "x", b: { text: 1, at: NOW }, c: { text: "", at: NOW } }), NOW)).toEqual({});
  });

  it("drops a draft older than a day, since its seat name may belong to another seat by now", () => {
    const old = { text: "old", at: NOW - KEEP_MS - 1 };
    const fresh = { text: "fresh", at: NOW - KEEP_MS };
    expect(parseDrafts(JSON.stringify({ old, fresh }), NOW)).toEqual({ fresh });
  });
});

describe("withDraft", () => {
  it("sets, replaces and removes one seat's draft", () => {
    const one = withDraft({}, "h/a", "first", NOW);
    expect(one["h/a"]?.text).toBe("first");
    expect(withDraft(one, "h/a", "second", NOW + 1)["h/a"]?.text).toBe("second");
    expect(withDraft(one, "h/a", "", NOW + 1)).toEqual({});
  });

  it("keeps other seats' drafts when one changes", () => {
    const both = withDraft(withDraft({}, "h/a", "a", NOW), "h/b", "b", NOW + 1);
    expect(Object.keys(withDraft(both, "h/a", "", NOW + 2))).toEqual(["h/b"]);
  });

  it("does not store a draft past the length cap", () => {
    expect(withDraft({}, "h/a", "x".repeat(MAX_CHARS + 1), NOW)).toEqual({});
    expect(withDraft({}, "h/a", "x".repeat(MAX_CHARS), NOW)["h/a"]?.text).toHaveLength(MAX_CHARS);
  });

  it("drops the oldest draft past the count cap", () => {
    let stored = {};
    for (let i = 0; i <= MAX_DRAFTS; i++) stored = withDraft(stored, `h/${i}`, `t${i}`, NOW + i);
    expect(Object.keys(stored)).toHaveLength(MAX_DRAFTS);
    expect(stored).not.toHaveProperty("h/0");
  });
});

describe("draftKey", () => {
  it("separates the same session name on two hosts", () => {
    expect(draftKey("mac", "imp")).not.toBe(draftKey("phone", "imp"));
  });
});
