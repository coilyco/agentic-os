import { describe, expect, it } from "vitest";
import fixture from "./fixtures/roster.json";
import { parseRoster, RosterError } from "./roster";

describe("parseRoster", () => {
  it("decodes the live aterm.roster.v1 snapshot", () => {
    const roles = parseRoster(fixture);
    const frontend = roles.find((role) => role.slug === "frontend-eng");
    expect(frontend?.identity).toBe("Imp-Dragonfly");
    expect(frontend?.color).toMatch(/^#[0-9a-f]{6}$/);
    expect(frontend?.seats.map((seat) => seat.key)).toContain("claude");
  });

  it("refuses another contract instead of guessing", () => {
    expect(() => parseRoster({ format: "aterm.roster.v2", roles: [] })).toThrow(RosterError);
  });

  it("refuses a colour that is not #rrggbb", () => {
    const bad = { format: "aterm.roster.v1", roles: [{ slug: "x", display_name: "X", favorite_color: "pink", identity: { name: "Y" }, seats: [] }] };
    expect(() => parseRoster(bad)).toThrow(/favorite_color/);
  });
});
