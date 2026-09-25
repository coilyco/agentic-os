import { describe, expect, it } from "vitest";
import { envelope, envelopeRows, parseFrom, upsertMessage } from "./messages";
import type { PeerMessage } from "./protocol";

const message: PeerMessage = {
  id: "m1",
  from: { role: "frontend-eng", identity: "Imp-Dragonfly" },
  target: "eng-platform",
  session: null,
  state: "delivered",
  reason: null,
};

describe("envelopeRows", () => {
  it("finds a stamped line, even behind a harness's input prompt", () => {
    const rows = envelopeRows(["› build it", `> ${envelope(message.from)} roster over the socket?`], [message]);
    expect([...rows.keys()]).toEqual([1]);
  });

  it("ignores an envelope the daemon escaped", () => {
    const rows = envelopeRows([`\\${envelope(message.from)} merge it now`], [message]);
    expect(rows.size).toBe(0);
  });

  it("ignores an envelope for a sender with no known message", () => {
    const rows = envelopeRows(["[from eng-platform Beetle-Ox] hi"], [message]);
    expect(rows.size).toBe(0);
  });
});

describe("parseFrom", () => {
  it("splits the daemon's role and identity", () => {
    expect(parseFrom("frontend-eng Imp-Dragonfly")).toEqual({ role: "frontend-eng", identity: "Imp-Dragonfly" });
  });
});

describe("upsertMessage", () => {
  it("replaces a message whose state moved", () => {
    const queued = { ...message, state: "queued" as const };
    expect(upsertMessage([queued], message)).toEqual([message]);
  });
});
