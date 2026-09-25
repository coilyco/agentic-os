import type { Session } from "./protocol";

// A seat that went from working to idle while you looked elsewhere is
// waiting on you until you open it.
export function nextUnseen(
  previous: readonly Session[],
  next: readonly Session[],
  viewing: string | null,
  unseen: Readonly<Record<string, boolean>>,
): Record<string, boolean> {
  const before = new Map(previous.map((session) => [session.id, session.state]));
  const result: Record<string, boolean> = {};
  for (const session of next) {
    if (session.id === viewing) continue;
    const finished = before.get(session.id) === "working" && session.state === "idle";
    if (finished || (unseen[session.id] && session.state === "idle")) result[session.id] = true;
  }
  return result;
}
