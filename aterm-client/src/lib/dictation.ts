// The phone's own speech recognition, called through the dictation plugin.
// Shapes: models.rs in the dictation plugin. Design: references/android-dictation.md.
import { bridge, type Invoke } from "./device-key";

const COMMAND = "plugin:dictation|";

export type Phase = "idle" | "listening" | "done" | "failed";
const PHASES: readonly Phase[] = ["idle", "listening", "done", "failed"];

/** What the recognizer holds. `seq` rises on every change. */
export interface Heard {
  phase: Phase;
  text: string;
  error: number | null;
  seq: number;
}

export function parseHeard(raw: unknown): Heard {
  const { phase, text, error, seq } = (raw ?? {}) as Record<string, unknown>;
  return {
    phase: PHASES.find((each) => each === phase) ?? "idle",
    text: typeof text === "string" ? text : "",
    error: typeof error === "number" ? error : null,
    seq: typeof seq === "number" ? seq : 0,
  };
}

/** The mic shows when a recognizer exists and the microphone is not refused. */
export function parseAvailable(raw: unknown): boolean {
  const { available, permission } = (raw ?? {}) as { available?: unknown; permission?: unknown };
  return available === true && permission !== "denied";
}

// Android's `SpeechRecognizer.ERROR_*` codes in words. Others say their number.
const ERRORS: Record<number, string> = {
  1: "The speech service timed out on the network. Try again.",
  2: "The speech service could not reach the network. Try again.",
  3: "The microphone gave no sound. Check that no other app holds it.",
  4: "The speech service failed. Try again.",
  6: "No speech heard. Tap the mic and talk.",
  7: "No words recognised. Tap the mic and try again.",
  8: "The speech service is busy. Try again in a moment.",
  9: "The microphone is not allowed for this app. Allow it in Android's app settings.",
  10: "The speech service is rate limited. Wait a moment, then try again.",
  12: "The speech service does not support this language.",
  13: "This language's speech pack is not on the phone yet.",
};

/** A reason in words for a stopped recognizer, or null when text came through anyway. */
export function errorWords(code: number | null, heard: string): string | null {
  // A pause after words can end with `no match`, and the words stay.
  if (heard && (code === 6 || code === 7)) return null;
  if (code === null) return "Dictation stopped without an answer. Try again.";
  return ERRORS[code] ?? `Dictation failed (Android error ${code}). Try again.`;
}

/** Words for a refused `start`, from its `{code, message}` rejection. */
export function refusalWords(error: unknown): string {
  const { code, message } = (error ?? {}) as { code?: unknown; message?: unknown };
  if (code === "denied") return "The microphone is not allowed for this app. Allow it in Android's app settings.";
  if (code === "unavailable") return "This phone has no speech recognition service.";
  return typeof message === "string" && message ? message : "Dictation would not start.";
}

/** The draft around where speech goes. Speech replaces a selection. */
export interface Anchor {
  before: string;
  after: string;
}

export function anchorAt(text: string, start: number, end: number): Anchor {
  const from = Math.max(0, Math.min(start, end, text.length));
  const to = Math.max(from, Math.min(Math.max(start, end), text.length));
  return { before: text.slice(0, from), after: text.slice(to) };
}

/** The draft with `words` placed, spaced from neighbours, and the caret after them. */
export function placeSpeech(anchor: Anchor, words: string): { text: string; caret: number } {
  const said = words.trim();
  if (!said) return { text: anchor.before + anchor.after, caret: anchor.before.length };
  const lead = anchor.before !== "" && !/\s$/.test(anchor.before) ? " " : "";
  const trail = anchor.after !== "" && !/^\s/.test(anchor.after) ? " " : "";
  const before = anchor.before + lead + said + trail;
  return { text: before + anchor.after, caret: (anchor.before + lead + said).length };
}

export async function dictationAvailable(invoke: Invoke | null = bridge()): Promise<boolean> {
  if (!invoke) return false;
  try {
    return parseAvailable(await invoke(`${COMMAND}available`));
  } catch {
    // An app built before the plugin rejects the call, which reads as no mic.
    return false;
  }
}

export interface DictationHandlers {
  /** Words so far, or the final words when `final`. The same text may come again. */
  onText: (words: string, final: boolean) => void;
  /** The session is over. `note` says why it ended badly, and is null otherwise. */
  onEnd: (note: string | null) => void;
}

export interface Session {
  /** Ask for the final words, which arrive through `onText`, then `onEnd`. */
  stop: () => Promise<void>;
  /** Drop the session and keep what was already placed. */
  cancel: () => Promise<void>;
}

export const POLL_MS = 150;
// A recognizer asked to stop gives its words within a second or two, or not at all.
export const STOP_GRACE_MS = 4000;

/** Starts one session. Rejects with words for the person if it would not start. */
export async function startDictation(
  handlers: DictationHandlers,
  invoke: Invoke | null = bridge(),
  language?: string,
): Promise<Session> {
  if (!invoke) throw new Error("Dictation is not available here.");
  try {
    await invoke(`${COMMAND}start`, { language: language ?? null });
  } catch (error) {
    throw new Error(refusalWords(error));
  }
  let last = -1;
  let heard = "";
  let over = false;
  let stopAt: number | null = null;
  let polling = false;
  let timer: ReturnType<typeof setInterval> | undefined;
  const finish = (note: string | null): void => {
    if (over) return;
    over = true;
    clearInterval(timer);
    handlers.onEnd(note);
  };
  const tick = async (): Promise<void> => {
    // A slow bridge must not stack polls, or an old answer replays after a new one.
    if (over || polling) return;
    polling = true;
    let now: Heard;
    try {
      now = parseHeard(await invoke(`${COMMAND}poll`));
    } catch {
      finish("Dictation lost contact with the speech service.");
      return;
    } finally {
      polling = false;
    }
    if (over) return;
    if (now.seq !== last) {
      last = now.seq;
      if (now.text) {
        heard = now.text;
        handlers.onText(now.text, now.phase !== "listening");
      }
    }
    if (now.phase === "done") finish(null);
    else if (now.phase === "failed") finish(errorWords(now.error, heard));
    else if (now.phase === "idle") finish(null);
    else if (stopAt !== null && Date.now() - stopAt > STOP_GRACE_MS) {
      void invoke(`${COMMAND}cancel`).catch(() => undefined);
      if (heard) handlers.onText(heard, true);
      finish(heard ? null : errorWords(6, ""));
    }
  };
  timer = setInterval(() => void tick(), POLL_MS);
  return {
    stop: async () => {
      if (over || stopAt !== null) return;
      stopAt = Date.now();
      try {
        await invoke(`${COMMAND}stop`);
      } catch {
        finish("Dictation could not stop cleanly.");
      }
    },
    cancel: async () => {
      if (over) return;
      finish(null);
      try {
        await invoke(`${COMMAND}cancel`);
      } catch {
        // The session is already over for the page.
      }
    },
  };
}
