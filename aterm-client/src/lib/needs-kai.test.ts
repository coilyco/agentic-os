import { describe, expect, it } from "vitest";
import { seatsNeedingKai } from "./needs-kai";
import type { Ask, SeatArtifact } from "./protocol";

const seat = (n: number) => ({ id: `role-seat-${n}`, role: "role", identity: `Seat-${n}` });
const ask = (session: string, extra: Partial<Ask> = {}): Ask => ({ id: `ask-${session}`, session, header: "h", question: "q", options: [], allowOther: false, multi: false, ...extra });
const page = (session: string, extra: Partial<SeatArtifact> = {}): SeatArtifact => ({ id: `page-${session}`, session, title: "A page", url: "https://claude.ai/artifact/x", html: "<p>x</p>", standing: false, waiting: true, seatEnded: false, ...extra });

describe("seatsNeedingKai", () => {
  const twenty = Array.from({ length: 20 }, (_, n) => seat(n));

  it("counts 3 for 20 idle seats, 2 on a choice card and 1 with a page waiting", () => {
    const found = seatsNeedingKai([ask("role-seat-3"), ask("role-seat-4")], [page("role-seat-9")], twenty);
    expect(found.map((each) => [each.identity, each.kind])).toEqual([["Seat-3", "asking"], ["Seat-4", "asking"], ["Seat-9", "artifact"]]);
  });

  it("is empty when every seat only finished its turn", () => {
    expect(seatsNeedingKai([], [], twenty)).toEqual([]);
  });

  it("names a seat once when it asks and has a page waiting, as asking", () => {
    const found = seatsNeedingKai([ask("role-seat-3")], [page("role-seat-3")], twenty);
    expect(found).toHaveLength(1);
    expect(found[0]?.kind).toBe("asking");
  });

  it("leaves out a page that is not waiting, and a page whose seat ended", () => {
    expect(seatsNeedingKai([], [page("role-seat-1", { waiting: false }), page("role-seat-2", { seatEnded: true })], twenty)).toEqual([]);
  });

  it("leaves out a page or a question whose seat is not in the roster", () => {
    expect(seatsNeedingKai([ask("gone")], [page("gone")], twenty)).toEqual([]);
  });

  it("puts a worker's relayed question on the director that shows its card", () => {
    const found = seatsNeedingKai([ask("role-seat-5", { relay: true, cardSeat: "role-seat-0" })], [], twenty);
    expect(found.map((each) => each.sessionId)).toEqual(["role-seat-0"]);
  });
});
