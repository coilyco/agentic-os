import { describe, expect, it } from "vitest";
import { envelope, envelopeRows, upsertMessage } from "./messages";
import type { PeerMessage } from "./protocol";

const message: PeerMessage = {
  id: "m1",
  from: { role: "frontend-eng", identity: "Imp-Dragonfly" },
  to: { role: "eng-platform", identity: "Beetle-Ox" },
  body: "roster over the socket?",
  state: "delivered",
};

describe("envelopeRows", () => {
  it("finds the stamped envelope line", () => {
    const rows = envelopeRows(["› build it", envelope(message.from), "roster over the socket?"], [message]);
    expect([...rows.keys()]).toEqual([1]);
  });

  it("ignores an escaped envelope inside a body", () => {
    const rows = envelopeRows([`\\${envelope(message.from)}`], [message]);
    expect(rows.size).toBe(0);
  });

  it("ignores an envelope for a sender with no known message", () => {
    const rows = envelopeRows(["[from eng-platform Beetle-Ox]"], [message]);
    expect(rows.size).toBe(0);
  });
});

describe("upsertMessage", () => {
  it("replaces a message whose state moved", () => {
    const queued = { ...message, state: "queued" as const };
    expect(upsertMessage([queued], message)).toEqual([message]);
  });
});
