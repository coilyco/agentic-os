// The sound cue: two soft Web Audio notes, no file shipped. A browser keeps audio
// suspended until a gesture, so `unlock` runs from the switch and the first touch.

type Listener = (blocked: boolean) => void;

let context: AudioContext | null = null;
let onBlocked: Listener = () => {};

export function watchBlocked(listener: Listener): void {
  onBlocked = listener;
}

function ensure(): AudioContext | null {
  if (context) return context;
  const Ctor = window.AudioContext ?? (window as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!Ctor) return null;
  context = new Ctor();
  context.onstatechange = () => onBlocked(context?.state !== "running");
  return context;
}

/** Wakes audio. True when the browser let it run. Call from a user gesture. */
export async function unlock(): Promise<boolean> {
  const ctx = ensure();
  if (!ctx) {
    onBlocked(true);
    return false;
  }
  try {
    await ctx.resume();
  } catch {
    // A refusal shows as a state that is not running, read below.
  }
  const running = ctx.state === "running";
  onBlocked(!running);
  return running;
}

/** Plays one cue. False, and the switch says so, while the browser blocks audio. */
export function chime(): boolean {
  const ctx = ensure();
  if (!ctx || ctx.state !== "running") {
    onBlocked(true);
    return false;
  }
  const start = ctx.currentTime;
  [660, 880].forEach((frequency, index) => {
    const at = start + index * 0.14;
    const tone = ctx.createOscillator();
    const gain = ctx.createGain();
    tone.type = "sine";
    tone.frequency.value = frequency;
    gain.gain.setValueAtTime(0.0001, at);
    gain.gain.exponentialRampToValueAtTime(0.18, at + 0.02);
    gain.gain.exponentialRampToValueAtTime(0.0001, at + 0.22);
    tone.connect(gain).connect(ctx.destination);
    tone.start(at);
    tone.stop(at + 0.24);
  });
  return true;
}

/** Whether audio is shut, so a reload with Sound on can say it awaits a touch. */
export function isBlocked(): boolean {
  const ctx = ensure();
  return !ctx || ctx.state !== "running";
}
