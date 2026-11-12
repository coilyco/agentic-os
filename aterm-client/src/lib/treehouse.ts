// Where this build finds Treehouse accounts and the broker, from build config and never
// source. A build that sets neither has no accounts surface and keeps the direct path.

export interface TreehouseConfig {
  accountsUrl: string;
  brokerUrl: string;
  /** Sign in on accounts' own page in the system browser, as an app must. */
  hop: boolean;
}

interface Env {
  VITE_TREEHOUSE_ACCOUNTS_URL?: string;
  VITE_TREEHOUSE_BROKER_URL?: string;
  VITE_TREEHOUSE_SIGNIN?: string;
  VITE_ATERM_APP?: string;
}

function origin(value: string | undefined): string | null {
  if (!value) return null;
  try {
    const url = new URL(value);
    return url.protocol === "https:" || url.protocol === "http:" ? url.origin : null;
  } catch {
    return null;
  }
}

/** Both addresses, or null: half a configuration is no configuration. */
export function treehouseConfig(env: Env = import.meta.env): TreehouseConfig | null {
  const accountsUrl = origin(env.VITE_TREEHOUSE_ACCOUNTS_URL);
  const brokerUrl = origin(env.VITE_TREEHOUSE_BROKER_URL);
  return accountsUrl && brokerUrl ? { accountsUrl, brokerUrl, hop: env.VITE_TREEHOUSE_SIGNIN === "hop" || env.VITE_ATERM_APP === "1" } : null;
}
