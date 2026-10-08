import { afterEach, describe, expect, it, vi } from "vitest";
import {
  CODE_SPENT, createCredential, creationOptions, credentialJson, fromBase64Url, getCredential, passkeyFailure, PasskeyError,
  passkeySupport, relyingParty, requestOptions, rpMismatch, toBase64Url,
} from "./passkey";

const bytes = (...values: number[]) => new Uint8Array(values);
const CREATION = {
  publicKey: {
    rp: { name: "aterm", id: "coilyco.dev" },
    user: { name: "kai", displayName: "Kai", id: toBase64Url(bytes(1, 2, 3)) },
    challenge: toBase64Url(bytes(250, 251, 252, 253)),
    pubKeyCredParams: [{ type: "public-key", alg: -7 }],
    excludeCredentials: [{ type: "public-key", id: toBase64Url(bytes(9, 8)), transports: ["internal"] }],
    authenticatorSelection: { userVerification: "required" },
  },
};
const REQUEST = { publicKey: { challenge: toBase64Url(bytes(7, 7, 7)), rpId: "coilyco.dev", userVerification: "required", allowCredentials: [{ type: "public-key", id: toBase64Url(bytes(5)) }] } };

describe("base64url", () => {
  it("round-trips bytes the standard alphabet would pad or spell with + and /", () => {
    for (const sample of [bytes(), bytes(251), bytes(251, 255), bytes(251, 255, 254), bytes(0, 1, 2, 3, 4, 5, 6, 250, 251, 252, 253, 254, 255)]) {
      const text = toBase64Url(sample);
      expect(text).not.toMatch(/[+/=]/);
      expect([...new Uint8Array(fromBase64Url(text))]).toEqual([...sample]);
    }
  });
});

describe("the options the daemon sends", () => {
  it("decodes the creation fields the browser wants as bytes, and leaves the rest alone", () => {
    const options = creationOptions(CREATION);
    expect([...new Uint8Array(options.challenge as ArrayBuffer)]).toEqual([250, 251, 252, 253]);
    expect([...new Uint8Array(options.user.id as ArrayBuffer)]).toEqual([1, 2, 3]);
    expect([...new Uint8Array(options.excludeCredentials![0]!.id as ArrayBuffer)]).toEqual([9, 8]);
    expect(options.excludeCredentials![0]).toMatchObject({ type: "public-key", transports: ["internal"] });
    expect(options.rp).toEqual({ name: "aterm", id: "coilyco.dev" });
    expect(options.authenticatorSelection).toEqual({ userVerification: "required" });
  });

  it("decodes the assertion fields, and has no allow list to decode when the daemon names none", () => {
    const options = requestOptions(REQUEST);
    expect([...new Uint8Array(options.challenge as ArrayBuffer)]).toEqual([7, 7, 7]);
    expect([...new Uint8Array(options.allowCredentials![0]!.id as ArrayBuffer)]).toEqual([5]);
    expect("allowCredentials" in requestOptions({ publicKey: { challenge: "AQ", rpId: "coilyco.dev" } })).toBe(false);
  });

  it("reads the relying party from either shape, and refuses options with no publicKey", () => {
    expect(relyingParty(CREATION)).toBe("coilyco.dev");
    expect(relyingParty(REQUEST)).toBe("coilyco.dev");
    expect(() => creationOptions({})).toThrow(PasskeyError);
    expect(() => requestOptions(null)).toThrow(/cannot read/);
  });

  it("matches a host to the relying party only as itself or a subdomain", () => {
    expect(rpMismatch("coilyco.dev", "coilyco.dev")).toBe(false);
    expect(rpMismatch("coilyco.dev", "app.coilyco.dev")).toBe(false);
    expect(rpMismatch("coilyco.dev", "kais-mac.tail1234.ts.net")).toBe(true);
    expect(rpMismatch("coilyco.dev", "evilcoilyco.dev")).toBe(true);
    expect(rpMismatch(undefined, "anything")).toBe(false);
  });
});

describe("credentialJson", () => {
  const shared = { id: "abc", rawId: bytes(1, 2).buffer, type: "public-key", getClientExtensionResults: () => ({}) };

  it("prefers the browser's own toJSON", () => {
    expect(credentialJson({ ...shared, toJSON: () => ({ id: "from-browser" }) } as unknown as PublicKeyCredential)).toEqual({ id: "from-browser" });
  });

  it("spells out a registration, with transports, when the browser has no toJSON", () => {
    const response = { clientDataJSON: bytes(3).buffer, attestationObject: bytes(8, 9).buffer, getTransports: () => ["internal"] };
    expect(credentialJson({ ...shared, response } as unknown as PublicKeyCredential)).toEqual({
      id: "abc", rawId: "AQI", type: "public-key", clientExtensionResults: {},
      response: { clientDataJSON: "Aw", attestationObject: "CAk", transports: ["internal"] },
    });
  });

  it("spells out an assertion, with a null user handle when there is none", () => {
    const response = { clientDataJSON: bytes(3).buffer, authenticatorData: bytes(5).buffer, signature: bytes(6).buffer, userHandle: null };
    expect(credentialJson({ ...shared, response } as unknown as PublicKeyCredential)).toMatchObject({
      response: { clientDataJSON: "Aw", authenticatorData: "BQ", signature: "Bg", userHandle: null },
    });
  });
});

describe("what the person reads when the browser ends the ceremony", () => {
  const named = (name: string) => Object.assign(new Error("x"), { name });

  it("names a closed prompt, a missing method, and a page the browser refuses, each differently", () => {
    const words = ["NotAllowedError", "AbortError", "InvalidStateError", "SecurityError", "NotSupportedError", "Other"].map((name) => passkeyFailure(named(name), "coilyco.dev"));
    expect(new Set(words).size).toBe(6);
    expect(words[3]).toContain("coilyco.dev");
    expect(words[0]).toMatch(/fingerprint, a face or a PIN/);
  });

  it("passes a message already in words straight through", () => {
    expect(passkeyFailure(new PasskeyError("Passkeys here belong to coilyco.dev."))).toBe("Passkeys here belong to coilyco.dev.");
  });
});

describe("running the ceremony in the browser", () => {
  afterEach(() => vi.unstubAllGlobals());
  const stub = (credentials: unknown, secure = true) => {
    vi.stubGlobal("navigator", { credentials });
    vi.stubGlobal("isSecureContext", secure);
  };
  const made = { id: "n", rawId: bytes(1).buffer, type: "public-key", toJSON: () => ({ id: "made" }) };

  it("says so before anything is sent when the browser has no passkeys or the page is not secure", () => {
    stub(undefined);
    expect(passkeySupport()).toMatch(/cannot make or use passkeys/);
    stub({ create: vi.fn(), get: vi.fn() }, false);
    expect(passkeySupport()).toMatch(/secure page/);
    stub({ create: vi.fn(), get: vi.fn() });
    expect(passkeySupport()).toBeNull();
  });

  it("creates with the decoded options and returns the credential as JSON", async () => {
    const create = vi.fn().mockResolvedValue(made);
    stub({ create, get: vi.fn() });
    await expect(createCredential(CREATION, "coilyco.dev")).resolves.toEqual({ id: "made" });
    expect(create.mock.calls[0]![0].publicKey.rp.id).toBe("coilyco.dev");
    expect(create.mock.calls[0]![0].publicKey.challenge).toBeInstanceOf(ArrayBuffer);
  });

  it("refuses before the prompt on a page that is not the relying party, and never calls the browser", async () => {
    const create = vi.fn();
    stub({ create, get: vi.fn() });
    await expect(createCredential(CREATION, "kais-mac.tail1234.ts.net")).rejects.toThrow(/belong to coilyco\.dev/);
    expect(create).not.toHaveBeenCalled();
  });

  it("turns a cancelled prompt into words, for create and for get", async () => {
    stub({ create: vi.fn().mockRejectedValue(named("NotAllowedError")), get: vi.fn().mockRejectedValue(named("NotAllowedError")) });
    await expect(createCredential(CREATION, "coilyco.dev")).rejects.toThrow(/closed or timed out/);
    await expect(getCredential(REQUEST, "coilyco.dev")).rejects.toThrow(/closed or timed out/);
  });

  it("asserts with the decoded options", async () => {
    const get = vi.fn().mockResolvedValue({ ...made, toJSON: () => ({ id: "asserted" }) });
    stub({ create: vi.fn(), get });
    await expect(getCredential(REQUEST, "coilyco.dev")).resolves.toEqual({ id: "asserted" });
    expect(get.mock.calls[0]![0].publicKey.allowCredentials[0].id).toBeInstanceOf(ArrayBuffer);
  });

  it("says the code is used up in the one sentence the enroll path appends", () => {
    expect(CODE_SPENT).toMatch(/aterm passkey enroll/);
  });
});

function named(name: string): Error {
  return Object.assign(new Error(name), { name });
}
