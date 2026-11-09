// The line a seat writes about what it is doing (COI-3339), and how old it is.
import { ageText } from "./age";

/** Past this, a line stops looking current and says how old it is. */
export const STALE_AFTER_S = 300;

export interface Activity {
  text: string;
  /** When the seat set it, in ms. */
  at: number;
}

/** The frame's fields as a line. No text, or no usable time, means no line. */
export function toActivity(text: string | undefined, at: string | undefined): Activity | undefined {
  const line = text?.trim();
  const when = at === undefined ? Number.NaN : Date.parse(at);
  return line && Number.isFinite(when) ? { text: line, at: when } : undefined;
}

/** Empty while the line is recent, else how long ago it was set: "12m ago". */
export function activityFlag(activity: Activity, now: number): string {
  const seconds = Math.max(0, (now - activity.at) / 1000);
  return seconds < STALE_AFTER_S ? "" : `${ageText(seconds)} ago`;
}

/** The line as a screen reader gets it, since a row's name replaces its text. */
export function activityWords(activity: Activity | undefined, now: number): string {
  if (!activity) return "";
  const flag = activityFlag(activity, now);
  return flag ? `says ${activity.text}, set ${flag}` : `says ${activity.text}`;
}
