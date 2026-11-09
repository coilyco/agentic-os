// Sending shared images to a seat: upload each, then paste the paths and the words.
// It copies Composer.svelte `send`, which another change held. They should become one.
import { attachNote, pasteSteps, withImagePaths } from "./images";
import type { ImageChannel } from "./protocol";

const PASTE_OPEN = "\x1b[200~";
const PASTE_CLOSE = "\x1b[201~";
const STEP_MS = 200;

export interface Target {
  id: string;
  seat: string;
  /** The program asked for bracketed paste, so a path arrives as an attachment. */
  paste: boolean;
}

export interface SendIo {
  input(sessionId: string, data: string): void;
  later(run: () => void, ms: number): void;
}

/** Uploads all files before the seat gets anything. Rejects in words. */
export async function sendToSeat(
  channel: Pick<ImageChannel, "upload">,
  io: SendIo,
  target: Target,
  words: string,
  files: readonly File[],
  progress: (done: number, total: number) => void = () => {},
): Promise<string | null> {
  const paths: string[] = [];
  for (const [index, file] of files.entries()) {
    progress(index, files.length);
    paths.push(await channel.upload(target.id, file));
  }
  const typed = words.trim();
  if (target.paste) {
    const steps = pasteSteps(paths, typed, PASTE_OPEN, PASTE_CLOSE);
    for (const [index, step] of steps.entries()) {
      if (index === 0) io.input(target.id, step);
      else io.later(() => io.input(target.id, step), index * STEP_MS);
    }
    io.later(() => io.input(target.id, "\r"), (steps.length - 1) * STEP_MS + 300);
  } else {
    io.input(target.id, `${withImagePaths(typed, paths).replace(/\r?\n/g, " ")}\r`);
  }
  return (paths.length && attachNote(target.seat)) || null;
}
