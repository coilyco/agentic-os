import { describe, expect, it } from "vitest";
import { groupOf, groupSeats } from "./overview";
import type { Session } from "./protocol";

const seat = (id: string, patch: Partial<Session> = {}): Session => ({ id, role: "r", seat: "claude", identity: id, state: "idle", pending: 0, drafting: false, paste: true, degraded: [], ...patch });
const none = { needs: new Set<string>(), unseen: {} };
const keys = (groups: ReturnType<typeof groupSeats>) => groups.map((group) => group.key);
const ids = (groups: ReturnType<typeof groupSeats>, key: string) => groups.find((group) => group.key === key)?.seats.map((each) => each.id);

describe("groupOf", () => {
  it("puts a seat with an open ask in needs you, whatever its state says", () => {
    const context = { ...none, needs: new Set(["a"]) };
    expect(groupOf(seat("a", { state: "working" }), context)).toBe("needs");
    expect(groupOf(seat("a", { state: "idle" }), context)).toBe("needs");
  });

  it("puts a working seat in working and an idle one in done", () => {
    expect(groupOf(seat("a", { state: "working" }), none)).toBe("working");
    expect(groupOf(seat("a", { state: "idle" }), none)).toBe("done");
  });

  it("keeps a failed launch out of working, so it reads as done", () => {
    expect(groupOf(seat("a", { state: "failed" }), none)).toBe("done");
  });
});

describe("groupSeats", () => {
  it("orders the groups needs you, working, done", () => {
    const groups = groupSeats([seat("d"), seat("w", { state: "working" }), seat("n")], { ...none, needs: new Set(["n"]) });
    expect(keys(groups)).toEqual(["needs", "working", "done"]);
    expect(groups.map((group) => group.title)).toEqual(["Needs you", "Working", "Done"]);
  });

  it("leaves an empty group out, so no heading sits over nothing", () => {
    expect(keys(groupSeats([seat("w", { state: "working" })], none))).toEqual(["working"]);
    expect(groupSeats([], none)).toEqual([]);
  });

  it("puts the seat that has waited longest first in needs you, and a seat with no reading last", () => {
    const groups = groupSeats([seat("new", { stateSince: 9000 }), seat("blank"), seat("old", { stateSince: 1000 })], { ...none, needs: new Set(["new", "blank", "old"]) });
    expect(ids(groups, "needs")).toEqual(["old", "new", "blank"]);
  });

  it("puts a reply Kai has not read before one she has, inside done", () => {
    const groups = groupSeats([seat("read", { stateSince: 1000 }), seat("unread", { stateSince: 9000 })], { ...none, unseen: { unread: true } });
    expect(ids(groups, "done")).toEqual(["unread", "read"]);
  });

  it("keeps roster order inside working, and for seats with no reading", () => {
    const groups = groupSeats([seat("b", { state: "working" }), seat("a", { state: "working" }), seat("y"), seat("x")], none);
    expect(ids(groups, "working")).toEqual(["b", "a"]);
    expect(ids(groups, "done")).toEqual(["y", "x"]);
  });

  it("does not reorder or mutate the list it was given", () => {
    const given = [seat("b", { stateSince: 9 }), seat("a", { stateSince: 1 })];
    groupSeats(given, { ...none, needs: new Set(["a", "b"]) });
    expect(given.map((each) => each.id)).toEqual(["b", "a"]);
  });
});
