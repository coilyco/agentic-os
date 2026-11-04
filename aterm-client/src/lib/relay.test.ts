import { describe, expect, it } from "vitest";
import { toAsk } from "./daemon-host";
import { askerLine, cardSeatOf } from "./relay";
import type { Ask } from "./protocol";

const ask = (over: Partial<Ask> = {}): Ask => ({ id: "a1", session: "worker", header: "", question: "Which?", options: [], allowOther: false, multi: false, ...over });
const live = new Set(["worker", "director"]);

describe("toAsk", () => {
  const view = { id: "a1", session: "worker", question: "Which?", options: [{ label: "One" }] };

  it("carries the relay and the seat that shows the card", () => {
    expect(toAsk({ ...view, relay: true, card_seat: "director" })).toMatchObject({ relay: true, cardSeat: "director" });
  });

  it("leaves both off an ask_choice, and off a relay with no director up", () => {
    expect(toAsk(view)).not.toHaveProperty("cardSeat");
    expect(toAsk({ ...view, relay: true, card_seat: "" })).not.toHaveProperty("cardSeat");
  });
});

describe("cardSeatOf", () => {
  it("is the asker for a plain ask", () => {
    expect(cardSeatOf(ask(), live)).toBe("worker");
  });

  it("is the director a relayed ask was sent to", () => {
    expect(cardSeatOf(ask({ relay: true, cardSeat: "director" }), live)).toBe("director");
  });

  it("falls back to the asker when that director is gone, so the card is never lost", () => {
    expect(cardSeatOf(ask({ relay: true, cardSeat: "gone" }), live)).toBe("worker");
  });
});

describe("askerLine", () => {
  const name = (id: string) => (id === "worker" ? "Frog-Ox" : id === "director" ? "Griffin-Goose" : id);

  it("says where an asker's question is shown", () => {
    expect(askerLine(ask({ relay: true, cardSeat: "director" }), "worker", live, name)).toBe("asking you, on Griffin-Goose");
  });

  it("says who is asking on the screen that shows it", () => {
    expect(askerLine(ask({ relay: true, cardSeat: "director" }), "director", live, name)).toBe("Frog-Ox asks you");
  });

  it("is plain on a seat asking for itself", () => {
    expect(askerLine(ask(), "worker", live, name)).toBe("asking you");
  });
});
