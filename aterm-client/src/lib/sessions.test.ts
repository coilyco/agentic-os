import { describe, expect, it } from "vitest";
import type { Host, Session } from "./protocol";
import type { Role } from "./roster";
import { hostRunning, orderSessions, roleFor, sessionCode, sessionLabel } from "./sessions";

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

describe("hostRunning", () => {
  const online = (id: string, sessionCount: number): Host => ({ id, label: id, address: id, kind: "daemon", status: { kind: "online", sessionCount } });
  const five = ["a", "b", "c", "d", "e"].map((id) => live(id, "scientist", "Frog-Ox"));

  it("follows a roster push that adds one session, so the tab and the Running heading agree", () => {
    const host = online("local", 5);
    expect(hostRunning(host, "local", five)).toBe(5);
    const pushed = [...five, live("f", "scientist", "Frog-Ox")];
    expect(hostRunning(host, "local", pushed)).toBe(6);
    expect(hostRunning(host, "local", pushed)).toBe(orderSessions(pushed, []).length);
  });

  it("drops with a close, and shows zero rather than the stale probe", () => {
    expect(hostRunning(online("local", 5), "local", [])).toBe(0);
  });

  it("keeps the probe for a host it is not attached to, and says nothing for one that is not online", () => {
    expect(hostRunning(online("saved:x", 3), "local", five)).toBe(3);
    expect(hostRunning({ ...online("local", 5), status: { kind: "checking" } }, "local", five)).toBeNull();
  });
});
