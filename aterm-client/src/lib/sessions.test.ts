import { describe, expect, it } from "vitest";
import type { Session } from "./protocol";
import type { Role } from "./roster";
import { orderSessions, roleFor, sessionCode, sessionLabel } from "./sessions";

function live(id: string, role: string, identity: string, seat = "claude"): Session {
  return { id, role, identity, seat, state: "idle", pending: 0, drafting: false, paste: true, degraded: [] };
}

function role(slug: string, displayName = slug): Role {
  return { slug, displayName, purpose: "", color: "#112233", identity: "", launchable: true, seats: [] };
}

describe("sessionCode", () => {
  it("reads the daemon's suffix, and nothing from an unsuffixed name", () => {
    expect(sessionCode(live("eng-platform-beetle-ox-gd85", "eng-platform", "Beetle-Ox"))).toBe("gd85");
    expect(sessionCode(live("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox"))).toBe("");
  });
});

describe("orderSessions", () => {
  it("keeps every instance, grouped in roster order", () => {
    const sessions = [
      live("scientist-frog-ox-va67", "scientist", "Frog-Ox"),
      live("eng-platform-beetle-ox-gd85", "eng-platform", "Beetle-Ox", "codex"),
      live("mystery-x-aa11", "mystery", "X"),
      live("eng-platform-beetle-ox-eb64", "eng-platform", "Beetle-Ox"),
    ];
    const ordered = orderSessions(sessions, [role("eng-platform"), role("scientist")]).map((each) => each.id);
    expect(ordered).toEqual(["eng-platform-beetle-ox-eb64", "eng-platform-beetle-ox-gd85", "scientist-frog-ox-va67", "mystery-x-aa11"]);
  });
});

describe("roleFor", () => {
  it("draws a role the roster does not list instead of dropping its session", () => {
    expect(roleFor(live("mystery-x", "mystery", "X"), [])).toMatchObject({ slug: "mystery", displayName: "mystery", identity: "X" });
  });
});

describe("sessionLabel", () => {
  it("names role, harness, code, state, and a degraded start", () => {
    const session = { ...live("eng-platform-beetle-ox-gd85", "eng-platform", "Beetle-Ox", "codex"), degraded: ["telemetry"] };
    expect(sessionLabel(session, role("eng-platform", "Platform Engineer"), "working")).toBe(
      "Beetle-Ox, Platform Engineer on codex, session gd85, working, started without telemetry",
    );
  });
});
