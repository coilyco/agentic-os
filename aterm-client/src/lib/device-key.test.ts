import { describe, expect, it, vi } from "vitest";
import { biometricNotice, bridge, DeviceKeyError, deviceKeyFailure, enrollKey, keyOffer, keyStatus, mayEnroll, parseEnrolled, parseSigned, parseStatus, signChallenge } from "./device-key";

const READY = { enrolled: true, keyId: "k1", biometric: "ready" as const };

describe("bridge", () => {
  it("finds the app's invoke, and is null in a browser or an app without it", () => {
    const invoke = vi.fn();
    expect(bridge({ __TAURI__: { core: { invoke } } })).toBe(invoke);
    expect(bridge({})).toBeNull();
    expect(bridge({ __TAURI__: {} })).toBeNull();
    expect(bridge({ __TAURI__: { core: { invoke: "no" } } })).toBeNull();
  });
});

describe("what the plugin returns", () => {
  it("reads the status, and treats an unreadable biometric as unsupported", () => {
    expect(parseStatus({ enrolled: true, key_id: "k1", biometric: "ready" })).toEqual(READY);
    expect(parseStatus({ enrolled: false, key_id: null, biometric: "none_enrolled" })).toEqual({ enrolled: false, keyId: null, biometric: "none_enrolled" });
    expect(parseStatus({ enrolled: true, key_id: "k1", biometric: "strange" }).biometric).toBe("unsupported");
    expect(parseStatus(null)).toEqual({ enrolled: false, keyId: null, biometric: "unsupported" });
  });

  it("refuses an enrollment or a signature missing a field, instead of sending half of one", () => {
    expect(parseEnrolled({ key_id: "k", public_key: "p", attestation: ["a"] })).toEqual({ keyId: "k", publicKey: "p", attestation: ["a"] });
    expect(() => parseEnrolled({ key_id: "k", public_key: "p" })).toThrow(DeviceKeyError);
    expect(parseSigned({ key_id: "k", signature: "s" })).toEqual({ keyId: "k", signature: "s" });
    expect(() => parseSigned({ key_id: "k", signature: "" })).toThrow(DeviceKeyError);
  });
});

describe("the attestation", () => {
  it("is the certificate chain, leaf first, forwarded as the plugin returned it", () => {
    const chain = ["MIIleaf", "MIIintermediate", "MIIroot"];
    expect(parseEnrolled({ key_id: "k", public_key: "p", attestation: chain }).attestation).toEqual(chain);
  });

  it("is refused when it is empty, holds a non-string, or is not a list", () => {
    for (const attestation of [[], ["ok", ""], ["ok", 7], "MIIone,MIItwo", null]) {
      expect(() => parseEnrolled({ key_id: "k", public_key: "p", attestation })).toThrow(DeviceKeyError);
    }
  });
});

describe("the plugin calls", () => {
  it("name the command, and pass the challenge and the prompt the plugin signs", async () => {
    const invoke = vi.fn().mockResolvedValueOnce({ enrolled: false, key_id: null, biometric: "ready" }).mockResolvedValueOnce({ key_id: "k", public_key: "p", attestation: ["a"] }).mockResolvedValueOnce({ key_id: "k", signature: "s" });
    await keyStatus(invoke);
    await enrollKey(invoke, "CHAL");
    await signChallenge(invoke, "CHAL2", "Unlock");
    expect(invoke.mock.calls).toEqual([["plugin:devicekey|status", undefined], ["plugin:devicekey|enroll", { challenge: "CHAL" }], ["plugin:devicekey|sign", { challenge: "CHAL2", prompt: "Unlock" }]]);
  });

  it("turn a plugin rejection into words for the person, whatever shape it arrives in", async () => {
    const reject = (error: unknown) => vi.fn().mockRejectedValue(error);
    await expect(signChallenge(reject({ code: "cancelled", message: "x" }), "c", "p")).rejects.toThrow(/closed the prompt/);
    await expect(signChallenge(reject({ code: "lockout" }), "c", "p")).rejects.toThrow(/Too many tries/);
    await expect(signChallenge(reject("raw native stack trace"), "c", "p")).rejects.toThrow(/^The phone stopped the fingerprint prompt\.$/);
    await expect(keyStatus(reject(new Error("boom")))).rejects.toThrow(DeviceKeyError);
  });
});

describe("the words for a phone that cannot ask", () => {
  it("name each state differently, and say nothing when the phone is ready", () => {
    const words = (["none_enrolled", "unsupported", "invalidated"] as const).map(biometricNotice);
    expect(new Set(words).size).toBe(3);
    expect(biometricNotice("ready")).toBeNull();
    expect(deviceKeyFailure({ code: "invalidated" })).toBe(biometricNotice("invalidated"));
  });

  it("let a phone with a changed fingerprint enroll again, but not one that cannot ask at all", () => {
    expect(mayEnroll(READY)).toBe(true);
    expect(mayEnroll({ ...READY, biometric: "invalidated" })).toBe(true);
    expect(mayEnroll({ ...READY, biometric: "none_enrolled" })).toBe(false);
    expect(mayEnroll({ ...READY, biometric: "unsupported" })).toBe(false);
  });
});

describe("keyOffer", () => {
  it("shows nothing until the phone has answered", () => {
    expect(keyOffer(null, null, false)).toBeNull();
  });

  it("offers the unlock button for an enrolled phone the host knows, with enrollment tucked away", () => {
    expect(keyOffer(READY, "enrolled", false)).toEqual({ blocked: null, unlock: true, enroll: true, enrollOpen: false });
    expect(keyOffer(READY, null, false)).toMatchObject({ unlock: true });
  });

  it("opens the enrollment steps when the phone has no key, the host has none, or the host forgot this one", () => {
    expect(keyOffer({ ...READY, enrolled: false, keyId: null }, "unenrolled", false)).toEqual({ blocked: null, unlock: false, enroll: true, enrollOpen: true });
    expect(keyOffer(READY, "unenrolled", false)).toMatchObject({ unlock: false, enrollOpen: true });
    expect(keyOffer(READY, "enrolled", true)).toMatchObject({ unlock: false, enrollOpen: true });
  });

  it("says why, with no control, when the phone cannot ask for a fingerprint", () => {
    const offer = keyOffer({ ...READY, biometric: "none_enrolled" }, null, false);
    expect(offer).toEqual({ blocked: biometricNotice("none_enrolled"), unlock: false, enroll: false, enrollOpen: false });
  });

  it("explains a changed fingerprint and still offers a new key, since the old one is unusable", () => {
    const offer = keyOffer({ ...READY, biometric: "invalidated" }, "enrolled", false);
    expect(offer).toEqual({ blocked: biometricNotice("invalidated"), unlock: false, enroll: true, enrollOpen: true });
  });
});
