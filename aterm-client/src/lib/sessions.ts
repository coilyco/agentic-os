// Live sessions as peers: every instance on every harness gets its own tab,
// so two Beetle-Ox sessions, or one on claude and one on codex, never fold into one.
import type { Host, Session } from "./protocol";
import type { Role } from "./roster";

/** The pool slot after `<role>-<identity>` ("2", "3"), or "" for the bare name. */
export function sessionCode(session: Pick<Session, "id" | "role" | "identity">): string {
  const prefix = `${session.role}-${session.identity.toLowerCase()}-`;
  return session.id.startsWith(prefix) ? session.id.slice(prefix.length) : "";
}

/** Roster order first, so a role's instances sit together, then by name. */
export function orderSessions(sessions: readonly Session[], roles: readonly Role[]): Session[] {
  const rank = new Map(roles.map((role, index) => [role.slug, index]));
  const place = (session: Session) => rank.get(session.role) ?? roles.length;
  return [...sessions].sort((a, b) => place(a) - place(b) || a.id.localeCompare(b.id));
}

/** A session whose role the roster does not list still gets a tab, drawn neutral. */
export function roleFor(session: Pick<Session, "role" | "identity" | "seat">, roles: readonly Role[]): Role {
  return (
    roles.find((role) => role.slug === session.role) ?? {
      slug: session.role,
      displayName: session.role,
      purpose: "",
      color: "#a4abb9",
      identity: session.identity,
      launchable: false,
      seats: [],
    }
  );
}

/** A session tab's accessible name, since identity repeats across instances. */
export function sessionLabel(session: Session, role: Role, activity: string): string {
  const code = sessionCode(session);
  const parts = [`${session.identity}, ${role.displayName} on ${session.seat}`];
  if (code) parts.push(`session ${code}`);
  parts.push(activity);
  if (session.degraded.length) parts.push(`started without ${session.degraded.join(", ")}`);
  return parts.join(", ");
}

/** Attached host: count the list the Running heading lists, not the connect probe. */
export function hostRunning(host: Host, attachedHostId: string | null, sessions: readonly Session[]): number | null {
  if (host.status.kind !== "online") return null;
  return host.id === attachedHostId ? sessions.length : host.status.sessionCount;
}
