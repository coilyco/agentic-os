// Sign-in and OAuth pages open in the device's own browser, never in a seat's streamed
// one: Cloudflare's check fails any browser driven over CDP by design (COI-3803).
import { signInUrl } from "./login";
import { treehouseConfig } from "./treehouse";

const CLAUDE_HOSTS = new Set(["claude.ai", "www.claude.ai", "claude.com", "www.claude.com"]);
// GitHub's login, device code, OAuth authorize and two-factor pages.
const GITHUB_SIGN_IN = /^\/(login(\/device(\/[^/]*)?|\/oauth\/authorize)?|session|sessions\/two-factor(\/[^/]*)?)\/?$/;

/** A Claude OAuth authorize page, the one a seat's `/login` opens. */
export function isClaudeSignIn(raw: string): boolean {
  if (signInUrl(raw)) return true;
  try {
    const { protocol, hostname, pathname } = new URL(raw);
    return protocol === "https:" && CLAUDE_HOSTS.has(hostname) && /\/oauth\/authorize\/?$/.test(pathname);
  } catch {
    return false;
  }
}

/** A page a person signs in on, which only the device's own browser can finish. */
export function isSignInPage(raw: string, accountsOrigin: string | null = treehouseConfig()?.accountsUrl ?? null): boolean {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return false;
  }
  if (accountsOrigin !== null && url.origin === accountsOrigin) return true;
  if (url.protocol !== "https:") return false;
  if (isClaudeSignIn(raw) || url.hostname === "accounts.google.com") return true;
  return url.hostname === "github.com" && GITHUB_SIGN_IN.test(url.pathname);
}
