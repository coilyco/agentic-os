/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_ATERM_DAEMON_WS?: string;
  readonly VITE_ATERM_HOSTED?: string;
  readonly VITE_ATERM_APP?: string;
  readonly VITE_ATERM_LOCAL_HOST?: string;
  readonly VITE_ATERM_LOCAL_LABEL?: string;
  readonly VITE_SENTRY_DSN?: string;
  /** Where Treehouse accounts and the broker answer. Set both or neither. */
  readonly VITE_TREEHOUSE_ACCOUNTS_URL?: string;
  readonly VITE_TREEHOUSE_BROKER_URL?: string;
  /** "hop" signs in on accounts' page in the system browser. An app build always does. */
  readonly VITE_TREEHOUSE_SIGNIN?: string;
}

declare const __ATERM_BUILD__: string;
