import { describe, expect, it } from "vitest";
import { nextUnseen } from "./activity";
import type { Session } from "./protocol";

function seat(id: string, state: Session["state"]): Session {
  return { id, role: id, seat: "claude", identity: id, state, pending: 0, drafting: false, paste: true };
}

describe("nextUnseen", () => {
  it("flags a seat that finished its turn off screen", () => {
    expect(nextUnseen([seat("a", "working")], [seat("a", "idle")], null, {})).toEqual({ a: true });
  });

  it("does not flag the seat you are looking at", () => {
    expect(nextUnseen([seat("a", "working")], [seat("a", "idle")], "a", {})).toEqual({});
  });

  it("clears the flag once the seat starts working again", () => {
    expect(nextUnseen([seat("a", "idle")], [seat("a", "working")], null, { a: true })).toEqual({});
  });

  it("drops a seat that went away", () => {
    expect(nextUnseen([seat("a", "idle")], [], null, { a: true })).toEqual({});
  });
});
