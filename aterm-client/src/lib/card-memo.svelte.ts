// A card's working state, kept outside the card so a rebuilt card picks up where it was.
import type { ChoiceOption } from "./choices";

export interface CardMemo {
  /** Which row's text box is open, or -1. */
  writing: number;
  text: string;
  picked: number[];
  /** The text box had the keyboard, so a rebuilt card gives it back. */
  focused: boolean;
}

const memos = new Map<string, CardMemo>();

/** One seat's cards are told apart by their options, so a new question starts clean. */
export function memoKey(seat: string, options: readonly ChoiceOption[]): string {
  return `${seat}|${options.map((option) => `${option.label}\u0001${option.freeText}`).join("\u0002")}`;
}

export function cardMemo(key: string, picked: number[]): CardMemo {
  const found = memos.get(key);
  if (found) return found;
  const fresh = $state<CardMemo>({ writing: -1, text: "", picked, focused: false });
  memos.set(key, fresh);
  return fresh;
}

/** A card was answered or went away, so none of it comes back with a later one. */
export function forgetCardMemos(seat: string): void {
  for (const key of [...memos.keys()]) if (key.startsWith(`${seat}|`)) memos.delete(key);
}

/** The rows a multi-select already shows ticked, which a card starts with. */
export const tickedRows = (options: readonly ChoiceOption[]): number[] => options.flatMap((option, index) => (option.checked ? [index] : []));
