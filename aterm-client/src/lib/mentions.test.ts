import { describe, expect, it } from "vitest";
import { findMention, insertMention, mentionMatches } from "./mentions";
import type { Session, SessionState } from "./protocol";

function live(id: string, role: string, identity: string, state: SessionState = "idle"): Session {
  return { id, role, identity, seat: "claude", state, pending: 0, drafting: false, paste: true, degraded: [] };
}

const sessions = [
  live("scientist-evie-cd12", "scientist", "Evie"),
  live("eng-platform-beetle-ox-gd85", "eng-platform", "Beetle-Ox"),
  live("eng-platform-beetle-ox-eb64", "eng-platform", "Beetle-Ox", "working"),
  live("eng-platform-angie-zz00", "eng-platform", "Angie", "failed"),
];

describe("findMention", () => {
  it("finds the token at the start, after a space, and after a newline", () => {
    expect(findMention("@eng-pl", 7)).toEqual({ start: 0, end: 7, query: "eng-pl" });
    expect(findMention("tell @eng-pl", 12)).toEqual({ start: 5, end: 12, query: "eng-pl" });
    expect(findMention("first\n@sci", 10)).toEqual({ start: 6, end: 10, query: "sci" });
  });

  it("offers everything for a bare @", () => {
    expect(findMention("ask @", 5)).toEqual({ start: 4, end: 5, query: "" });
  });

  it("ignores an @ inside a word, such as an address", () => {
    expect(findMention("mail a@b.example", 16)).toBeNull();
  });

  it("ignores text with no token at the caret", () => {
    expect(findMention("@eng-pl done", 12)).toBeNull();
    expect(findMention("plain", 5)).toBeNull();
  });

  it("takes the whole token when the caret is inside it", () => {
    expect(findMention("@eng-platform later", 4)).toEqual({ start: 0, end: 13, query: "eng" });
  });
});

describe("mentionMatches", () => {
  it("offers the live eng-platform sessions for @eng-pl, in name order", () => {
    expect(mentionMatches(sessions, "eng-pl").map((each) => each.id)).toEqual(["eng-platform-beetle-ox-eb64", "eng-platform-beetle-ox-gd85"]);
  });

  it("matches an identity prefix and ignores case", () => {
    expect(mentionMatches(sessions, "BEETLE").map((each) => each.id)).toEqual(["eng-platform-beetle-ox-eb64", "eng-platform-beetle-ox-gd85"]);
  });

  it("never offers a session that failed to start", () => {
    expect(mentionMatches(sessions, "eng-platform-angie")).toEqual([]);
    expect(mentionMatches(sessions, "").map((each) => each.id)).not.toContain("eng-platform-angie-zz00");
  });

  it("offers every live session for an empty prefix, and none for a stranger", () => {
    expect(mentionMatches(sessions, "")).toHaveLength(3);
    expect(mentionMatches(sessions, "nobody")).toEqual([]);
  });

  it("does not reorder the list it was given", () => {
    const before = sessions.map((each) => each.id);
    mentionMatches(sessions, "");
    expect(sessions.map((each) => each.id)).toEqual(before);
  });
});

describe("insertMention", () => {
  it("swaps the token for the full name and a space, without the @", () => {
    const found = findMention("tell @eng-pl", 12)!;
    expect(insertMention("tell @eng-pl", found, "eng-platform-beetle-ox-gd85")).toEqual({
      text: "tell eng-platform-beetle-ox-gd85 ",
      caret: 33,
    });
  });

  it("keeps the words after the token, and the space that was already there", () => {
    const found = findMention("@eng-pl then stop", 7)!;
    const next = insertMention("@eng-pl then stop", found, "eng-platform-beetle-ox-gd85");
    expect(next.text).toBe("eng-platform-beetle-ox-gd85 then stop");
    expect(next.caret).toBe(28);
  });

  it("replaces the whole token when the caret was inside it", () => {
    const found = findMention("@eng-platform later", 4)!;
    expect(insertMention("@eng-platform later", found, "eng-platform-beetle-ox-gd85").text).toBe("eng-platform-beetle-ox-gd85 later");
  });
});
