import { describe, expect, it } from "vitest";
import { keepStanding, needsPasskey, parseTyping, passkeyOffer, refusalOf, TYPING_OPEN, typingNotice, type Typing } from "./typing";

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
    const words = ["session_descendant", "peer_unread", "passkey_required", "something_new"].map((reason) => typingNotice({ allowed: false, reason }));
    expect(new Set(words).size).toBe(4);
    expect(words[3]).toContain("something_new");
  });

  it("says the app cannot unlock yet, instead of pointing at a page it cannot reach", () => {
    const locked: Typing = { allowed: false, reason: "passkey_required", passkey: "unenrolled" };
    expect(typingNotice(locked, "app")).toMatch(/app cannot unlock it yet/);
    expect(typingNotice(locked, "app")).not.toMatch(/coilyco\.dev|passkey/);
  });

  it("points a locked device at the hosted client unless this is the hosted client", () => {
    const locked: Typing = { allowed: false, reason: "passkey_required", passkey: "enrolled" };
    expect(typingNotice(locked, "hosted")).toMatch(/unlock it with a passkey/);
    expect(typingNotice(locked, "hosted")).not.toMatch(/coilyco\.dev/);
    expect(typingNotice(locked, "served")).toMatch(/hosted client at coilyco\.dev/);
  });
});

describe("the passkey standing", () => {
  it("reads enrolled or unenrolled from a locked device, and drops anything else", () => {
    expect(parseTyping({ allowed: false, reason: "passkey_required", passkey: "unenrolled" })).toEqual({ allowed: false, reason: "passkey_required", passkey: "unenrolled" });
    expect(parseTyping({ allowed: false, reason: "passkey_required", passkey: "maybe" })).toEqual({ allowed: false, reason: "passkey_required" });
    expect(parseTyping({ allowed: true, passkey: "enrolled" })).toEqual({ allowed: true, passkey: "enrolled" });
    expect(parseTyping({ allowed: true, passkey: "maybe" })).toEqual(TYPING_OPEN);
  });

  it("takes passkey_required from an error frame as a refusal", () => {
    expect(refusalOf({ reason: "passkey_required" })).toEqual({ allowed: false, reason: "passkey_required" });
  });

  it("keeps the standing a refusal frame leaves out, and only for the same reason", () => {
    const welcome: Typing = { allowed: false, reason: "passkey_required", passkey: "enrolled" };
    expect(keepStanding({ allowed: false, reason: "passkey_required" }, welcome)).toEqual(welcome);
    expect(keepStanding({ allowed: false, reason: "session_descendant" }, welcome)).toEqual({ allowed: false, reason: "session_descendant" });
    expect(keepStanding({ allowed: false, reason: "passkey_required", passkey: "unenrolled" }, welcome)).toEqual({ allowed: false, reason: "passkey_required", passkey: "unenrolled" });
    expect(keepStanding(TYPING_OPEN, welcome)).toEqual(TYPING_OPEN);
    expect(keepStanding({ allowed: false, reason: "passkey_required" }, undefined)).toEqual({ allowed: false, reason: "passkey_required" });
  });

  it("says a passkey is what lifts the lock only for passkey_required", () => {
    expect(needsPasskey({ allowed: false, reason: "passkey_required" })).toBe(true);
    expect(needsPasskey({ allowed: false, reason: "session_descendant" })).toBe(false);
    expect(needsPasskey(TYPING_OPEN)).toBe(false);
  });
});

describe("passkeyOffer", () => {
  const on = { hosted: true, channel: true };
  const locked = (passkey?: "enrolled" | "unenrolled"): Typing => ({ allowed: false, reason: "passkey_required", ...(passkey ? { passkey } : {}) });

  it("offers nothing where this page cannot run a ceremony, locked or not", () => {
    expect(passkeyOffer(locked("enrolled"), { hosted: false, channel: true })).toBeNull();
    expect(passkeyOffer(locked("enrolled"), { hosted: true, channel: false })).toBeNull();
    expect(passkeyOffer(locked("enrolled"), { hosted: true, channel: false, always: true })).toBeNull();
  });

  it("leads with the unlock for an enrolled device and with the code steps for an unenrolled one", () => {
    expect(passkeyOffer(locked("enrolled"), on)).toEqual({ unlock: true, enroll: true, enrollOpen: false });
    expect(passkeyOffer(locked("unenrolled"), on)).toEqual({ unlock: false, enroll: true, enrollOpen: true });
  });

  it("shows both paths when a refusal left the standing unknown, instead of none", () => {
    expect(passkeyOffer(locked(), on)).toEqual({ unlock: true, enroll: true, enrollOpen: true });
  });

  it("stays quiet on an unlocked device unless the host page asks for enrollment to stay reachable", () => {
    expect(passkeyOffer({ allowed: true, passkey: "enrolled" }, on)).toBeNull();
    expect(passkeyOffer({ allowed: true, passkey: "enrolled" }, { ...on, always: true })).toEqual({ unlock: false, enroll: true, enrollOpen: false });
    expect(passkeyOffer(TYPING_OPEN, { ...on, always: true })).toEqual({ unlock: false, enroll: true, enrollOpen: true });
  });

  it("never offers a passkey for a lock a passkey cannot lift", () => {
    expect(passkeyOffer({ allowed: false, reason: "session_descendant" }, on)).toBeNull();
  });
});
