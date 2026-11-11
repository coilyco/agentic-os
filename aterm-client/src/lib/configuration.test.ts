import { describe, expect, it } from "vitest";
import { rolesState, rowOf } from "./configuration";
import type { Role } from "./roster";

const role = (extra: Partial<Role> = {}): Role => ({
  slug: "eng-platform",
  displayName: "Platform Engineer",
  purpose: "",
  color: "#709b62",
  identity: "Beetle-Ox",
  launchable: true,
  seats: [
    { key: "claude", harness: "claude", name: "Beetle-Ox", tier: "frontier" },
    { key: "codex", harness: "codex", name: "Beetle-Ox", tier: "frontier" },
    { key: "claude-2", harness: "claude", name: "Beetle-Ox", tier: "" },
  ],
  ...extra,
});

describe("rolesState", () => {
  it("says no host when none is attached, whatever roles are left over", () => {
    expect(rolesState(false, false, [role()])).toEqual({ kind: "no-host" });
  });

  it("says no roles when an attached host has sent none", () => {
    expect(rolesState(true, true, [])).toEqual({ kind: "no-roles" });
  });

  it("lists the roles of a host that answers", () => {
    const state = rolesState(true, true, [role()]);
    expect(state).toMatchObject({ kind: "roles", stale: false });
  });

  it("keeps the last roles of a host that stopped answering, marked stale", () => {
    expect(rolesState(true, false, [role()])).toMatchObject({ kind: "roles", stale: true });
  });
});

describe("rowOf", () => {
  it("names the role, the seat, the pool and each client once", () => {
    expect(rowOf(role({ alternates: ["Stag-Ox", "Weevil-Ox"] }))).toEqual({
      slug: "eng-platform",
      name: "Platform Engineer",
      seat: "Beetle-Ox",
      pool: ["Stag-Ox", "Weevil-Ox"],
      color: "#709b62",
      clients: ["claude", "codex"],
    });
  });

  it("has an empty pool when the host lists none", () => {
    expect(rowOf(role()).pool).toEqual([]);
  });
});
