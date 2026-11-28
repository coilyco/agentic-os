// The words a dead seat's row carries in the sidebar (COI-3799).
import { ageText } from "./age";

/** Role, then how long ago the daemon found the seat's holder gone. */
export function deadDetail(roleName: string, endedAt: number, now: number): string {
  const age = endedAt > 0 ? ageText(Math.max(0, (now - endedAt) / 1000)) : "";
  return `${roleName} // found dead ${age ? `${age} ago` : "just now"}`;
}
