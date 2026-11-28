import type { ViewPick } from "./seat-view";

/** The open seat's view, shared by the phone bar and the desktop header. */
export const viewChoice = $state<{
  current: ViewPick | null;
  pick: ((choice: ViewPick) => void) | null;
  /** The host sends a transcript, so Focus, Default and Verbose can show. */
  levels: boolean;
}>({ current: null, pick: null, levels: false });
