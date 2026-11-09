// Feedback: a note and screenshots sent from the screen they are about. Daemon half:
// COI-3345, and `FeedbackChannel` in protocol.ts is the seam.
import { fitImage, formatSize, screenImages } from "./images";
import type { FeedbackContext, FeedbackChannel } from "./protocol";

/** The daemon's limit on a note, in characters. */
export const FEEDBACK_TEXT_MAX = 8000;

/** What the client knows about the screen at the moment the sheet opens. */
export interface Scene {
  host: string;
  seat: { identity: string; role: string; state: string } | null;
  /** A role picked to start, when no seat is on screen. */
  starting: string | null;
  place: string;
  build: string;
}

export function viewName(scene: Pick<Scene, "seat" | "starting">): string {
  if (scene.seat) return `Seat ${scene.seat.identity} (${scene.seat.role}, ${scene.seat.state})`;
  if (scene.starting) return `Start screen for ${scene.starting}`;
  return "Host list";
}

export function contextFor(scene: Scene, screen: { width: number; height: number; ratio: number }, now: Date): FeedbackContext {
  return { view: viewName(scene), host: scene.host, viewport: screen, build: scene.build, app: scene.place === "app" || scene.place === "app-key", at: now.toISOString() };
}

/** The line the sheet shows, so Kai sees what goes along without being asked for it. */
export function contextLine(context: FeedbackContext): string {
  const { width, height } = context.viewport;
  const where = context.app ? "Android app" : "browser";
  return `${context.view} // ${width}x${height} // ${where}, build ${context.build}`;
}

/** The daemon's refusal in words, or null for a reason this client has no wording for. */
export function feedbackRefusal(reason: unknown): string | null {
  switch (reason) {
    case "text_too_long":
      return `The host takes a note of ${FEEDBACK_TEXT_MAX.toLocaleString("en-US")} characters at most.`;
    case "passkey_required":
      return "This device has not unlocked typing yet, so the host would not take it.";
    case "session_descendant":
      return "The host does not take feedback from inside a seat.";
    default:
      return null;
  }
}

export interface Prepared {
  ready: File[];
  /** Why a file was left out or could not be sent, in words a person can act on. */
  refusal: string | null;
  /** Said, not refused: an image was shrunk to fit. */
  info: string | null;
}

/** Screens files against the host's limit and shrinks a still image that is over it. */
export async function prepareScreenshots(files: readonly File[], waiting: number, channel: Pick<FeedbackChannel, "maxBytes">): Promise<Prepared> {
  const screened = screenImages(files, waiting, channel);
  const ready: File[] = [];
  const shrunk: string[] = [];
  let refusal = screened.refusal;
  for (const file of screened.accepted) {
    if (file.size <= channel.maxBytes) {
      ready.push(file);
      continue;
    }
    const fitted = await fitImage(file, channel.maxBytes);
    if (fitted) {
      ready.push(fitted);
      shrunk.push(`${file.name || "A screenshot"} went from ${formatSize(file.size)} to ${formatSize(fitted.size)}`);
    } else {
      refusal ??= `${file.name || "That screenshot"} is ${formatSize(file.size)} and could not be shrunk under ${formatSize(channel.maxBytes)}.`;
    }
  }
  return { ready, refusal, info: shrunk.length ? `Shrunk to fit the ${formatSize(channel.maxBytes)} limit. ${shrunk.join(". ")}.` : null };
}

/** Whether Send has anything to send: a note, or a screenshot that says it. */
export function canSend(text: string, screenshots: number): boolean {
  return text.trim().length > 0 || screenshots > 0;
}

/** The status line while a send runs. */
export function sendingLine(done: number, total: number): string {
  if (total === 0) return "Sending your note.";
  if (done >= total) return "Sending your note.";
  return `Sending screenshot ${done + 1} of ${total}.`;
}
