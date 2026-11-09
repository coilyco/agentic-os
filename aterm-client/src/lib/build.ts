// What kind of page this build is, fixed at build time. See deploy.md.

/** The coilyco.dev build. */
export const HOSTED = import.meta.env.VITE_ATERM_HOSTED === "1";
/** The Android app, which bundles the client and serves it from its own origin. */
export const IN_APP = import.meta.env.VITE_ATERM_APP === "1";
/** Not served by the daemon it talks to. Hosts are added by name. */
export const EXTERNAL = HOSTED || IN_APP;
/** Only the coilyco.dev page can run the ceremony, since the daemon's RP is there. */
export const CAN_PASSKEY = HOSTED && !IN_APP;

export type Place = "hosted" | "app" | "app-key" | "served";
export const PLACE: Place = IN_APP ? "app" : HOSTED ? "hosted" : "served";
