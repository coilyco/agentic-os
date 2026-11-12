// The client half of the Treehouse accounts server: passkey sign-up and sign-in, the
// session, and unlock tokens. The session lives only in this closure, so closing ends it.
import { createCredential, getCredential, PasskeyError } from "./passkey";

export interface AccountsSession {
  accountId: string;
  name: string;
  accessToken: string;
  /** Milliseconds since the epoch, from the reply's expires_in. */
  accessExpiresAt: number;
}

export interface UnlockGrant {
  token: string;
  expiresIn: number;
}

export interface OwnedDaemon {
  id: string;
  name: string;
  substrate: string;
  created: string;
}

/** A treehouse.error.v1 reply, or a network failure with status 0. */
export class AccountsError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly status: number,
    readonly retryAfter?: number,
  ) {
    super(message);
  }
}

export interface AccountsOptions {
  /** The accounts origin, such as https://accounts.example.tech. */
  baseUrl: string;
  fetch?: typeof fetch;
  now?: () => number;
  /** The page host the passkey ceremony checks the relying party against. */
  hostname?: string;
}

/** Renew this long before expiry, so a request never carries a dying token. */
const RENEW_BEFORE_MS = 60_000;

interface SessionReply {
  access_token: string;
  expires_in: number;
  refresh_token: string;
  account_id: string;
  name: string;
}

interface Ceremony {
  ceremony: string;
  options: unknown;
}

export class AccountsClient {
  #session: AccountsSession | null = null;
  #refreshToken: string | null = null;
  // Refresh tokens work once and a spent one ends the session: callers share one request.
  #refreshing: Promise<AccountsSession> | null = null;
  readonly #fetch: typeof fetch;
  readonly #now: () => number;
  readonly #base: string;
  readonly #hostname: string | undefined;

  constructor(options: AccountsOptions) {
    this.#base = options.baseUrl.replace(/\/+$/, "");
    this.#fetch = options.fetch ?? ((...args) => fetch(...args));
    this.#now = options.now ?? Date.now;
    this.#hostname = options.hostname;
  }

  /** Who is signed in, or null. */
  get session(): AccountsSession | null {
    return this.#session;
  }

  async signUp(name: string): Promise<AccountsSession> {
    const begin = await this.#post<Ceremony>("/v1/auth/register/begin", { name });
    const credential = await this.#ceremony(() => createCredential({ publicKey: begin.options }, this.#hostname));
    return this.#start(await this.#post<SessionReply>("/v1/auth/register/finish", { ceremony: begin.ceremony, credential }));
  }

  async signIn(): Promise<AccountsSession> {
    const begin = await this.#post<Ceremony>("/v1/auth/login/begin", {});
    const credential = await this.#ceremony(() => getCredential({ publicKey: begin.options }, this.#hostname));
    return this.#start(await this.#post<SessionReply>("/v1/auth/login/finish", { ceremony: begin.ceremony, credential }));
  }

  /** Trades the hop's one-use code for this window's session. Refusals: code_invalid. */
  async handoff(code: string, verifier: string): Promise<AccountsSession> {
    return this.#start(await this.#post<SessionReply>("/v1/auth/handoff", { code, verifier }));
  }

  /** An access token with at least a minute left, renewed first when it has less. */
  async accessToken(): Promise<string> {
    const session = this.#session;
    if (!session) throw new AccountsError("unauthorized", "Sign in first.", 401);
    if (session.accessExpiresAt - this.#now() > RENEW_BEFORE_MS) return session.accessToken;
    return (await this.refresh()).accessToken;
  }

  refresh(): Promise<AccountsSession> {
    if (this.#refreshing) return this.#refreshing;
    const token = this.#refreshToken;
    if (!token) return Promise.reject(new AccountsError("unauthorized", "Sign in first.", 401));
    this.#refreshing = this.#post<SessionReply>("/v1/auth/refresh", { refresh_token: token })
      .then((reply) => this.#start(reply))
      .catch((error) => {
        // Any refusal ends the session, and a retry with the spent token would repeat it.
        if (error instanceof AccountsError && error.status >= 400 && error.status < 500) this.#clear();
        throw error;
      })
      .finally(() => {
        this.#refreshing = null;
      });
    return this.#refreshing;
  }

  /** Ends the session here and at accounts. Gone locally even if the request fails. */
  async signOut(): Promise<void> {
    const session = this.#session;
    this.#clear();
    if (!session) return;
    await this.#request("POST", "/v1/auth/logout", undefined, session.accessToken).catch(() => undefined);
  }

  /** Unlocks one channel of one daemon with the session, which needs no new prompt. */
  async unlockBySession(daemonId: string, nonce: string): Promise<UnlockGrant> {
    return this.#grant(await this.#authed<{ token: string; expires_in: number }>("POST", "/v1/unlock/finish", { daemon_id: daemonId, nonce, method: "session" }));
  }

  /** Unlocks one channel with a fresh passkey, for a session no longer user-verified. */
  async unlockByPasskey(daemonId: string, nonce: string): Promise<UnlockGrant> {
    const begin = await this.#authed<{ method: string; options: unknown }>("POST", "/v1/unlock/begin", { daemon_id: daemonId, nonce, method: "passkey" });
    const credential = await this.#ceremony(() => getCredential({ publicKey: begin.options }, this.#hostname));
    return this.#grant(await this.#authed<{ token: string; expires_in: number }>("POST", "/v1/unlock/finish", { daemon_id: daemonId, nonce, method: "passkey", credential }));
  }

  /** An enrollment code to type into `aterm` on the box. */
  async mintEnrollmentCode(): Promise<{ code: string; expiresIn: number }> {
    const reply = await this.#authed<{ code: string; expires_in: number }>("POST", "/v1/daemons/codes");
    return { code: reply.code, expiresIn: reply.expires_in };
  }

  async daemons(): Promise<OwnedDaemon[]> {
    return (await this.#authed<{ daemons: OwnedDaemon[] }>("GET", "/v1/daemons")).daemons;
  }

  async removeDaemon(id: string): Promise<void> {
    await this.#authed("DELETE", `/v1/daemons/${encodeURIComponent(id)}`);
  }

  #grant(reply: { token: string; expires_in: number }): UnlockGrant {
    return { token: reply.token, expiresIn: reply.expires_in };
  }

  #start(reply: SessionReply): AccountsSession {
    this.#refreshToken = reply.refresh_token;
    this.#session = {
      accountId: reply.account_id,
      name: reply.name,
      accessToken: reply.access_token,
      accessExpiresAt: this.#now() + reply.expires_in * 1000,
    };
    return this.#session;
  }

  #clear(): void {
    this.#session = null;
    this.#refreshToken = null;
  }

  async #ceremony<T>(run: () => Promise<T>): Promise<T> {
    try {
      return await run();
    } catch (error) {
      if (error instanceof PasskeyError) throw new AccountsError("passkey", error.message, 0);
      throw error;
    }
  }

  async #authed<T>(method: string, path: string, body?: unknown): Promise<T> {
    return this.#request<T>(method, path, body, await this.accessToken());
  }

  #post<T>(path: string, body: unknown): Promise<T> {
    return this.#request<T>("POST", path, body);
  }

  async #request<T>(method: string, path: string, body?: unknown, token?: string): Promise<T> {
    const headers: Record<string, string> = {};
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (token) headers.Authorization = `Bearer ${token}`;
    let response: Response;
    try {
      response = await this.#fetch(this.#base + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
    } catch {
      throw new AccountsError("network", "Accounts could not be reached.", 0);
    }
    const text = await response.text();
    const json = text ? (JSON.parse(text) as Record<string, unknown>) : {};
    if (!response.ok) {
      const retry = typeof json.retry_after === "number" ? json.retry_after : undefined;
      throw new AccountsError(String(json.error ?? "bad_request"), String(json.message ?? "Accounts refused the request."), response.status, retry);
    }
    return json as T;
  }
}
