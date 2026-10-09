// Stop: in Send's place while a seat works and the field is empty. A tap is Escape, which
// interrupts a turn. Holding is Control-C, for what Escape does not stop.

export const ESCAPE = "\x1b";
export const INTERRUPT = "\x03";
/** Long enough that a tap never reads as a hold. */
export const HOLD_MS = 600;

export interface StopInputs {
  working: boolean;
  hasText: boolean;
  /** Send was just pressed, and its confirmation has the button for a moment. */
  justSent: boolean;
  offline: boolean;
}

/** Whether the button is Stop. A draft keeps it Send, so a tap cannot lose it. */
export function showStop({ working, hasText, justSent, offline }: StopInputs): boolean {
  return working && !hasText && !justSent && !offline;
}
