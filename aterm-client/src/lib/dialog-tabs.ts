// The tab bar a multi-question dialog opens with: its tabs, and which is current.
export interface DialogTab {
  label: string;
  answered: boolean;
  submit: boolean;
  /** The column of its mark, which is where its highlight starts. */
  col: number;
}

export interface TabReading {
  tabs: DialogTab[];
  /** Index of the current tab, or -1 when the screen does not say. */
  current: number;
}

const MARK = "[☐☒✔✓■□]";
const TAB = new RegExp(`(${MARK})\\s+(.+?)(?=\\s{2,}${MARK}|\\s*→\\s*$|\\s*$)`, "gu");

export function parseTabBar(line: string): DialogTab[] | null {
  if (!new RegExp(`^\\s*←\\s+${MARK}`, "u").test(line)) return null;
  const tabs = [...line.matchAll(TAB)].map((match) => ({
    label: match[2]!.trim(),
    answered: match[1] === "☒",
    submit: /^submit$/i.test(match[2]!.trim()),
    col: match.index!,
  }));
  return tabs.length >= 2 ? tabs : null;
}

/** The tabs, and the current one: the tab whose mark sits on a cell with a background. */
export function readTabs(line: string, highlighted: ReadonlySet<number>): TabReading | null {
  const tabs = parseTabBar(line);
  if (!tabs) return null;
  return { tabs, current: tabs.findIndex((tab) => highlighted.has(tab.col)) };
}

const LEFT = "\x1b[D";
const RIGHT = "\x1b[C";

/** Key chunks to reach tab `to`. The arrows stop at the ends, so left finds the first. */
export function tabKeys(from: number, to: number, count: number): string[] {
  if (from >= 0) {
    if (to === from) return [];
    return [(to > from ? RIGHT : LEFT).repeat(Math.abs(to - from))];
  }
  return [LEFT.repeat(count), ...(to > 0 ? [RIGHT.repeat(to)] : [])];
}

/** "Question 2 of 3", "Review" on the Submit tab, or nothing when it is not known. */
export function progressText(reading: TabReading): string {
  const tab = reading.tabs[reading.current];
  if (!tab) return "";
  if (tab.submit) return "Review";
  return `Question ${reading.current + 1} of ${reading.tabs.filter((each) => !each.submit).length}`;
}
