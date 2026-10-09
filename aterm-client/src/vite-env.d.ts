/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_ATERM_DAEMON_WS?: string;
  readonly VITE_ATERM_HOSTED?: string;
  readonly VITE_ATERM_APP?: string;
  readonly VITE_SENTRY_DSN?: string;
}
