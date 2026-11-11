// What the Configuration Roles tab shows, from the roster the host already sends
// (COI-3348). Read only: nothing here writes to the host.
import type { Role } from "./roster";

export interface RoleRow {
  slug: string;
  name: string;
  /** The name a seat of this role takes now. */
  seat: string;
  /** The names later seats take, in the order the host lists them. */
  pool: string[];
  color: string;
  /** The harnesses a seat of this role can run on, each once. */
  clients: string[];
}

export type RolesState =
  | { kind: "no-host" }
  | { kind: "no-roles" }
  | { kind: "roles"; rows: RoleRow[]; stale: boolean };

/** `online` is a host answering. One that stopped keeps its last roles, marked stale. */
export function rolesState(attached: boolean, online: boolean, roles: Role[]): RolesState {
  if (!attached) return { kind: "no-host" };
  if (roles.length === 0) return { kind: "no-roles" };
  return { kind: "roles", rows: roles.map(rowOf), stale: !online };
}

export function rowOf(role: Role): RoleRow {
  return {
    slug: role.slug,
    name: role.displayName,
    seat: role.identity,
    pool: role.alternates ?? [],
    color: role.color,
    clients: [...new Set(role.seats.map((seat) => seat.harness))],
  };
}
