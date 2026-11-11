// Whether the sheet is open and what it was opened over. The note lives in the sheet.
import { app, inDemo, sessionById } from "./app.svelte";
import { PLACE } from "./build";
import { contextFor, feedbackMissing } from "./feedback";
import type { FeedbackContext } from "./protocol";

export const sheet = $state({ open: false, context: null as FeedbackContext | null });

export type FeedbackAvailability = { ok: true } | { ok: false; reason: string };

/** Whether Send can work from here, and in words why not. */
export function feedbackAvailability(): FeedbackAvailability {
  if (!app.connection) return { ok: false, reason: "Connect to a host first." };
  // The demo has no host to send to, and says so rather than blaming the build.
  if (inDemo()) return { ok: false, reason: "Feedback is off in the demo." };
  // Until the welcome names its features the host has not said, so nothing is claimed.
  if (!app.feedbackHost.arrived) return { ok: false, reason: "Connect to a host first." };
  if (!app.feedbackHost.has || !app.connection.feedback) return { ok: false, reason: feedbackMissing(app.feedbackHost.version) };
  return { ok: true };
}

/** Opens the sheet over the screen as it is now, since that is what the note is about. */
export function openFeedback(): void {
  const session = sessionById(app.selectedSession);
  const host = app.hosts.find((each) => each.id === app.attachedHostId);
  const scene = {
    host: host?.label ?? "unknown host",
    seat: session ? { identity: session.identity, role: session.role, state: session.state } : null,
    starting: session ? null : app.selectedRole,
    place: PLACE,
    build: __ATERM_BUILD__,
  };
  const screen = { width: window.innerWidth, height: window.innerHeight, ratio: window.devicePixelRatio || 1 };
  sheet.context = contextFor(scene, screen, new Date());
  sheet.open = true;
}
