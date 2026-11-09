// A card's working state, kept outside the card so a rebuilt card picks up where it was.
import type { ChoiceOption } from "./choices";

export interface CardMemo {
  /** Which row's text box is open, or -1. */
  writing: number;
  text: string;
  picked: number[];
  /** The text box had the keyboard, so a rebuilt card gives it back. */
  focused: boolean;
  /** A person folded the card to a bar to read the output behind it. Phones only. */
  collapsed: boolean;
}

const memos = new Map<string, CardMemo>();

/** A scope: `screen:<seat>` for a screen-read dialog, `ask:<id>` for a daemon ask. */
export const screenScope = (seat: string): string => `screen:${seat}`;
export const askScope = (askId: string): string => `ask:${askId}`;

/** Cards in one scope are told apart by their options, so a new question starts clean. */
export function memoKey(scope: string, options: readonly ChoiceOption[]): string {
  return `${scope}|${options.map((option) => `${option.label}\u0001${option.freeText}`).join("\u0002")}`;
}

export function cardMemo(key: string, picked: number[]): CardMemo {
  const found = memos.get(key);
  if (found) return found;
  const fresh = $state<CardMemo>({ writing: -1, text: "", picked, focused: false, collapsed: false });
  memos.set(key, fresh);
  return fresh;
}

/** A card was answered or went away, so none of it comes back. Other scopes stay. */
export function forgetCardMemos(scope: string): void {
  for (const key of [...memos.keys()]) if (key.startsWith(`${scope}|`)) memos.delete(key);
}

/** The rows a multi-select already shows ticked, which a card starts with. */
export const tickedRows = (options: readonly ChoiceOption[]): number[] => options.flatMap((option, index) => (option.checked ? [index] : []));
