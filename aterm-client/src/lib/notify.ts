// A coordinator seat's ping (COI-3517): played once per id, shown as a notice.
import type { Notify } from "./protocol";

const KEEP = 64;

/** Remembers the ids it has seen, newest last, so a doubled frame plays once. */
export class SeenIds {
  private ids: string[] = [];

  /** True the first time an id is offered. */
  first(id: string): boolean {
    if (this.ids.includes(id)) return false;
    this.ids.push(id);
    if (this.ids.length > KEEP) this.ids.shift();
    return true;
  }
}

/** Whether the page shows its own notification. A page the person is looking at, on the
 * seat that pinged, has the chime and needs no pop-up over its own screen. */
export function wantsDesktopNotice(seen: { focused: boolean; openSeat: string | null }, note: Notify): boolean {
  return !(seen.focused && seen.openSeat === note.session);
}

/** The notification's words: the seat's title and body, one tag per seat. */
export function noticeFor(note: Notify): { title: string; options: { body: string; tag: string; silent: boolean; data: { session: string } } } {
  return { title: note.title, options: { body: note.body, tag: `aterm-${note.session}`, silent: true, data: { session: note.session } } };
}
