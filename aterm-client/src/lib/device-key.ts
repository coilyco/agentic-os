// The app's unlock: a key held by the phone's Keystore behind the fingerprint prompt.
// The plugin is the app's. This reads what it returns and says what went wrong.

/** Whether the phone can ask for a fingerprint or screen lock, per the plugin. */
export type Biometric = "ready" | "none_enrolled" | "unsupported" | "invalidated";

export interface KeyStatus {
  enrolled: boolean;
  keyId: string | null;
  biometric: Biometric;
}

export interface EnrolledKey {
  keyId: string;
  publicKey: string;
  /** The certificate chain, leaf first, as the plugin returned it. */
  attestation: string[];
}

export interface Signed {
  keyId: string;
  signature: string;
}

/** A failure in words. `reason` names the daemon's refusal, when it was one. */
export class DeviceKeyError extends Error {
  constructor(message: string, readonly reason?: string) {
    super(message);
  }
}

/** The plugin's own call, present only inside the app. */
export type Invoke = (command: string, args?: Record<string, unknown>) => Promise<unknown>;

/** The page's bridge to the plugin, or null in a browser and in an app without it. */
export function bridge(scope: unknown = globalThis): Invoke | null {
  const invoke = (scope as { __TAURI__?: { core?: { invoke?: unknown } } }).__TAURI__?.core?.invoke;
  return typeof invoke === "function" ? (invoke as Invoke) : null;
}

const BIOMETRICS: readonly Biometric[] = ["ready", "none_enrolled", "unsupported", "invalidated"];
const str = (value: unknown): string | null => (typeof value === "string" && value !== "" ? value : null);
const COMMAND = "plugin:devicekey|";

export function parseStatus(raw: unknown): KeyStatus {
  const { enrolled, key_id, biometric } = (raw ?? {}) as { enrolled?: unknown; key_id?: unknown; biometric?: unknown };
  // An unreadable answer is the cautious one: no prompt to rely on.
  const known = BIOMETRICS.find((each) => each === biometric) ?? "unsupported";
  return { enrolled: enrolled === true, keyId: str(key_id), biometric: known };
}

export function parseEnrolled(raw: unknown): EnrolledKey {
  const { key_id, public_key, attestation } = (raw ?? {}) as Record<string, unknown>;
  const keyId = str(key_id);
  const publicKey = str(public_key);
  const chain = Array.isArray(attestation) && attestation.length > 0 && attestation.every((cert) => str(cert) !== null) ? (attestation as string[]) : null;
  if (!keyId || !publicKey || !chain) throw new DeviceKeyError("The phone made a key the app could not read.");
  return { keyId, publicKey, attestation: chain };
}

export function parseSigned(raw: unknown): Signed {
  const { key_id, signature } = (raw ?? {}) as Record<string, unknown>;
  const keyId = str(key_id);
  const proof = str(signature);
  if (!keyId || !proof) throw new DeviceKeyError("The phone answered the prompt in a way the app could not read.");
  return { keyId, signature: proof };
}

/** What a person reads when the phone cannot ask, or null when it can. */
export function biometricNotice(biometric: Biometric): string | null {
  switch (biometric) {
    case "none_enrolled":
      return "Set up a fingerprint or screen lock in Android settings, then come back.";
    case "unsupported":
      return "This phone has no fingerprint or screen lock the app can use.";
    case "invalidated":
      return "A fingerprint was added, so the key stopped working. Set it up again with a new code.";
    default:
      return null;
  }
}

/** The plugin rejects with a code and a message. Anything else is a plain stop. */
export function deviceKeyFailure(error: unknown): string {
  const code = (error as { code?: unknown } | null)?.code;
  switch (code) {
    case "cancelled":
      return "You closed the prompt, so nothing was unlocked.";
    case "lockout":
      return "Too many tries. Wait a moment, or unlock the phone with its screen lock, then try again.";
    case "none_enrolled":
    case "unsupported":
    case "invalidated":
      return biometricNotice(code) ?? "";
    default:
      return "The phone stopped the fingerprint prompt.";
  }
}

/** The failure after the daemon spent a code, so the person knows to ask for another. */
export const KEY_CODE_SPENT = "That code is used up. Run `aterm passkey enroll` again for a new one.";

async function call(invoke: Invoke, command: string, args?: Record<string, unknown>): Promise<unknown> {
  try {
    return await invoke(`${COMMAND}${command}`, args);
  } catch (error) {
    if (error instanceof DeviceKeyError) throw error;
    throw new DeviceKeyError(deviceKeyFailure(error));
  }
}

export async function keyStatus(invoke: Invoke): Promise<KeyStatus> {
  return parseStatus(await call(invoke, "status"));
}

export async function enrollKey(invoke: Invoke, challenge: string): Promise<EnrolledKey> {
  return parseEnrolled(await call(invoke, "enroll", { challenge }));
}

export async function signChallenge(invoke: Invoke, challenge: string, prompt: string): Promise<Signed> {
  return parseSigned(await call(invoke, "sign", { challenge, prompt }));
}

/** A phone that can ask may enroll. One that cannot would only waste the code. */
export function mayEnroll(status: KeyStatus): boolean {
  return status.biometric === "ready" || status.biometric === "invalidated";
}

/** What the unlock section offers. `standing` is the daemon's word, null if unsaid. */
export interface KeyOffer {
  /** A sentence in place of any control, because nothing here can help yet. */
  blocked: string | null;
  unlock: boolean;
  enroll: boolean;
  enrollOpen: boolean;
}

export function keyOffer(status: KeyStatus | null, standing: "enrolled" | "unenrolled" | null, unknownToDaemon: boolean): KeyOffer | null {
  if (status === null) return null;
  const notice = biometricNotice(status.biometric);
  if (notice && !mayEnroll(status)) return { blocked: notice, unlock: false, enroll: false, enrollOpen: false };
  const canUnlock = status.enrolled && status.biometric === "ready" && standing !== "unenrolled" && !unknownToDaemon;
  return { blocked: notice, unlock: canUnlock, enroll: true, enrollOpen: !canUnlock };
}
