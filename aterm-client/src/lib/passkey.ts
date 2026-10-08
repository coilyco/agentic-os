// The browser half of the passkey ceremony: the daemon's WebAuthn JSON in, the
// browser's credential back out as JSON. The relying party is the daemon's to name.

/** The daemon spends an enrollment code when it begins, so a later failure loses it. */
export const CODE_SPENT = "That code is used up. Run `aterm passkey enroll` again for a new one.";

/** A failure already in words for the person, from the browser or the options. */
export class PasskeyError extends Error {}

export function toBase64Url(bytes: ArrayBuffer | Uint8Array): string {
  const view = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  let binary = "";
  for (const byte of view) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function fromBase64Url(text: string): ArrayBuffer {
  const padded = text.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(text.length / 4) * 4, "=");
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index);
  return bytes.buffer;
}

interface Descriptor {
  id: string;
  type: string;
  transports?: string[];
}

const descriptors = (list: unknown): PublicKeyCredentialDescriptor[] | undefined =>
  Array.isArray(list) ? (list as Descriptor[]).map((each) => ({ ...each, id: fromBase64Url(each.id) }) as PublicKeyCredentialDescriptor) : undefined;

function publicKeyOf(raw: unknown): Record<string, unknown> {
  const publicKey = (raw as { publicKey?: unknown } | null)?.publicKey;
  if (typeof publicKey !== "object" || publicKey === null) throw new PasskeyError("The daemon sent passkey options this client cannot read.");
  return publicKey as Record<string, unknown>;
}

/** The library's `credentials.create` JSON, with its base64url fields decoded. */
export function creationOptions(raw: unknown): PublicKeyCredentialCreationOptions {
  const key = publicKeyOf(raw);
  const user = key.user as { id: string } & Record<string, unknown>;
  const exclude = descriptors(key.excludeCredentials);
  return {
    ...key,
    challenge: fromBase64Url(String(key.challenge)),
    user: { ...user, id: fromBase64Url(user.id) },
    ...(exclude ? { excludeCredentials: exclude } : {}),
  } as unknown as PublicKeyCredentialCreationOptions;
}

/** The library's `navigator.credentials.get` JSON, with its base64url fields decoded. */
export function requestOptions(raw: unknown): PublicKeyCredentialRequestOptions {
  const key = publicKeyOf(raw);
  const allow = descriptors(key.allowCredentials);
  return { ...key, challenge: fromBase64Url(String(key.challenge)), ...(allow ? { allowCredentials: allow } : {}) } as unknown as PublicKeyCredentialRequestOptions;
}

/** The relying party the options name, for a check before anything is sent or spent. */
export function relyingParty(raw: unknown): string | undefined {
  const key = publicKeyOf(raw) as { rp?: { id?: string }; rpId?: string };
  return key.rp?.id ?? key.rpId;
}

/** A page can only use a relying party that is its own host or a parent of it. */
export function rpMismatch(rpId: string | undefined, hostname: string): boolean {
  return Boolean(rpId) && hostname !== rpId && !hostname.endsWith(`.${rpId}`);
}

/** The credential as the daemon reads it: `toJSON()` if the browser has one. */
export function credentialJson(credential: PublicKeyCredential): unknown {
  if (typeof credential.toJSON === "function") return credential.toJSON();
  const response = credential.response;
  const shared = { id: credential.id, rawId: toBase64Url(credential.rawId), type: credential.type, clientExtensionResults: credential.getClientExtensionResults() };
  const clientDataJSON = toBase64Url(response.clientDataJSON);
  if ("attestationObject" in response) {
    const attestation = response as AuthenticatorAttestationResponse;
    return { ...shared, response: { clientDataJSON, attestationObject: toBase64Url(attestation.attestationObject), transports: attestation.getTransports?.() ?? [] } };
  }
  const assertion = response as AuthenticatorAssertionResponse;
  return {
    ...shared,
    response: { clientDataJSON, authenticatorData: toBase64Url(assertion.authenticatorData), signature: toBase64Url(assertion.signature), userHandle: assertion.userHandle ? toBase64Url(assertion.userHandle) : null },
  };
}

/** What a person reads when the browser, not the daemon, ends the ceremony. */
export function passkeyFailure(error: unknown, rpId?: string): string {
  if (error instanceof PasskeyError) return error.message;
  const name = error instanceof DOMException || error instanceof Error ? error.name : "";
  switch (name) {
    case "NotAllowedError":
      return "The passkey prompt was closed or timed out, or this device has nothing that can verify you, such as a fingerprint, a face or a PIN.";
    case "AbortError":
      return "The passkey prompt was cancelled.";
    case "InvalidStateError":
      return "This device already holds a passkey for this host.";
    case "SecurityError":
      return `The browser would not use a passkey from this page${rpId ? `, which is not part of ${rpId}` : ""}.`;
    case "NotSupportedError":
      return "This device has no passkey method the browser can use.";
    default:
      return `The browser stopped the passkey prompt${name ? ` (${name})` : ""}.`;
  }
}

/** Why this page cannot run a ceremony at all, or null. Check before a code is sent. */
export function passkeySupport(): string | null {
  if (typeof navigator === "undefined" || !navigator.credentials?.create || !navigator.credentials?.get) return "This browser cannot make or use passkeys.";
  if (typeof isSecureContext !== "undefined" && !isSecureContext) return "Passkeys need a secure page, and this one is not.";
  return null;
}

function ready(rpId: string | undefined, hostname: string): void {
  const unsupported = passkeySupport();
  if (unsupported) throw new PasskeyError(unsupported);
  if (rpMismatch(rpId, hostname)) throw new PasskeyError(`Passkeys here belong to ${rpId}. Open the client from there to use one.`);
}

/** Creates the passkey the daemon asked for, as the JSON the daemon reads. */
export async function createCredential(raw: unknown, hostname = location.hostname): Promise<unknown> {
  const rpId = relyingParty(raw);
  ready(rpId, hostname);
  try {
    const credential = await navigator.credentials.create({ publicKey: creationOptions(raw) });
    if (!credential) throw new PasskeyError("The browser made no passkey.");
    return credentialJson(credential as PublicKeyCredential);
  } catch (error) {
    throw new PasskeyError(passkeyFailure(error, rpId));
  }
}

/** Asserts the passkey the daemon asked for, and returns the assertion as JSON. */
export async function getCredential(raw: unknown, hostname = location.hostname): Promise<unknown> {
  const rpId = relyingParty(raw);
  ready(rpId, hostname);
  try {
    const credential = await navigator.credentials.get({ publicKey: requestOptions(raw) });
    if (!credential) throw new PasskeyError("The browser returned no passkey.");
    return credentialJson(credential as PublicKeyCredential);
  } catch (error) {
    throw new PasskeyError(passkeyFailure(error, rpId));
  }
}
