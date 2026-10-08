import { describe, expect, it } from "vitest";
import { parseTyping, refusalOf, TYPING_OPEN, typingNotice } from "./typing";

describe("parseTyping", () => {
  it("reads the welcome answer, and says nothing for a daemon without the guard", () => {
    expect(parseTyping({ allowed: true })).toEqual(TYPING_OPEN);
    expect(parseTyping({ allowed: false, reason: "session_descendant" })).toEqual({ allowed: false, reason: "session_descendant" });
    expect(parseTyping(undefined)).toBeNull();
    expect(parseTyping(null)).toBeNull();
  });

  it("fails closed on a refusal with no reason, and ignores a malformed answer", () => {
    expect(parseTyping({ allowed: false })).toEqual({ allowed: false, reason: "unspecified" });
    expect(parseTyping({ allowed: "no" })).toBeNull();
    expect(parseTyping("blocked")).toBeNull();
  });
});

describe("refusalOf", () => {
  it("takes the two accepted reasons from an error frame", () => {
    expect(refusalOf({ reason: "session_descendant" })).toEqual({ allowed: false, reason: "session_descendant" });
    expect(refusalOf({ reason: "peer_unread" })).toEqual({ allowed: false, reason: "peer_unread" });
  });

  it("leaves every other error alone, so an unrelated reason never locks the composer", () => {
    expect(refusalOf({})).toBeNull();
    expect(refusalOf({ reason: "no live session" })).toBeNull();
    expect(refusalOf({ reason: 7 })).toBeNull();
  });
});

describe("typingNotice", () => {
  it("is empty while typing is allowed", () => {
    expect(typingNotice(TYPING_OPEN)).toBe("");
  });

  it("names each accepted reason differently, and an unknown one with its code", () => {
    const words = ["session_descendant", "peer_unread", "passkey_required"].map((reason) => typingNotice({ allowed: false, reason }));
    expect(new Set(words).size).toBe(3);
    expect(words[2]).toContain("passkey_required");
  });
});
