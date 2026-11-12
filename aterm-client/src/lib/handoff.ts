// The app half of the sign-in hop (references/system-browser.md). The system browser runs
// the ceremony on accounts and sends back a code. The verifier stays here.

/** Where the apps are sent back to: the RFC 8252 private-use scheme of the Android id. */
export const APP_RETURN = "dev.coilyco.treehouse:/signin";

/** What a person reads when a return cannot be used. The cause is not theirs to fix. */
export const NOT_EXPECTED = "The sign-in did not come back as expected, so nothing was signed in. Try again.";

export class HandoffError extends Error {}

export interface Handoff {
  /** The page to open in the system browser. */
  url: string;
  /** Kept here until the code comes back. It is never in the URL. */
  verifier: string;
  state: string;
}

const base64Url = (bytes: Uint8Array) => btoa(String.fromCharCode(...bytes)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
const random = (length: number) => base64Url(crypto.getRandomValues(new Uint8Array(length)));

/** The base64url SHA-256 of the verifier, the only form of it that leaves the app. */
export async function challengeOf(verifier: string): Promise<string> {
  return base64Url(new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier))));
}

/** A fresh verifier, state and challenge, and the accounts page carrying the last two. */
export async function startHandoff(accountsUrl: string, redirectUri: string): Promise<Handoff> {
  // 32 random bytes are 43 characters, the contract's shortest verifier.
  const verifier = random(32);
  const state = random(16);
  const query = new URLSearchParams({ redirect_uri: redirectUri, state, challenge: await challengeOf(verifier) });
  return { url: `${accountsUrl.replace(/\/+$/, "")}/signin?${query}`, verifier, state };
}

/** The code in a return whose state is ours. Anything else is refused. */
export function readReturn(returnUrl: string, state: string): string {
  let params: URLSearchParams;
  try {
    params = new URL(returnUrl).searchParams;
  } catch {
    throw new HandoffError(NOT_EXPECTED);
  }
  const code = params.get("code");
  if (!code || params.get("state") !== state) throw new HandoffError(NOT_EXPECTED);
  return code;
}

/** A page load that may be a return: it carries both a code and a state. */
export function isHandoffReturn(search: string): boolean {
  const params = new URLSearchParams(search);
  return params.has("code") && params.has("state");
}
