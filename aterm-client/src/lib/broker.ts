// The client half of the Treehouse broker's API: which of the account's daemons are
// in, and a one-use ticket to one. The access token comes from accounts.

export interface BrokerDaemon {
  id: string;
  name: string;
  substrate: string;
  state: "online" | "offline";
  /** Seconds since the epoch, absent when the broker has never seen it. */
  lastSeen?: number;
  channels: number;
}

export interface Ticket {
  /** Opens once, within `expiresIn` seconds. */
  url: string;
  expiresIn: number;
}

/** A treehouse.error.v1 reply, or a network failure with status 0. */
export class BrokerError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly status: number,
    readonly retryAfter?: number,
  ) {
    super(message);
  }
}

export interface BrokerOptions {
  /** The broker origin, such as https://broker.example.tech. */
  baseUrl: string;
  /** A token with time left, renewed by accounts when it has little. */
  accessToken: () => Promise<string>;
  fetch?: typeof fetch;
}

interface ListRow {
  daemon_id: string;
  name: string;
  substrate: string;
  state: "online" | "offline";
  last_seen?: number;
  channels: number;
}

export class BrokerClient {
  readonly #base: string;
  readonly #token: () => Promise<string>;
  readonly #fetch: typeof fetch;

  constructor(options: BrokerOptions) {
    this.#base = options.baseUrl.replace(/\/+$/, "");
    this.#token = options.accessToken;
    this.#fetch = options.fetch ?? ((...args) => fetch(...args));
  }

  /** Only this account's daemons, online or not. Empty means the account has none. */
  async daemons(): Promise<BrokerDaemon[]> {
    const reply = await this.#request<{ daemons: ListRow[] }>("GET", "/v1/daemons");
    return reply.daemons.map((row) => ({
      id: row.daemon_id,
      name: row.name,
      substrate: row.substrate,
      state: row.state,
      channels: row.channels,
      ...(row.last_seen === undefined ? {} : { lastSeen: row.last_seen }),
    }));
  }

  /** A ticket for one daemon: 404 if not the account's, 503 if not dialed in. */
  async connect(daemonId: string): Promise<Ticket> {
    const reply = await this.#request<{ url: string; expires_in: number }>("POST", "/v1/connect", { daemon_id: daemonId });
    return { url: reply.url, expiresIn: reply.expires_in };
  }

  async #request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { Authorization: `Bearer ${await this.#token()}` };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    let response: Response;
    try {
      response = await this.#fetch(this.#base + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
    } catch {
      throw new BrokerError("network", "The broker could not be reached.", 0);
    }
    const text = await response.text();
    let json: Record<string, unknown> = {};
    try {
      json = text ? (JSON.parse(text) as Record<string, unknown>) : {};
    } catch {
      // A proxy's error page is a refusal, not a parse error to throw at a person.
      if (response.ok) throw new BrokerError("bad_request", "The broker sent a reply this client cannot read.", response.status);
    }
    if (!response.ok) {
      const retry = typeof json.retry_after === "number" ? json.retry_after : undefined;
      throw new BrokerError(String(json.error ?? "bad_request"), String(json.message ?? "The broker refused the request."), response.status, retry);
    }
    return json as T;
  }
}
