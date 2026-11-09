import { describe, expect, it } from "vitest";
import { parseLastOpen, seatToRestore } from "./last-open";

describe("parseLastOpen", () => {
  it("reads a host and a seat", () => {
    expect(parseLastOpen('{"host":"local","seat":{"id":"a-b","who":"Imp-Dragonfly"}}')).toEqual({ host: "local", seat: { id: "a-b", who: "Imp-Dragonfly" } });
  });

  it("reads a host with no seat, which is the host page", () => {
    expect(parseLastOpen('{"host":"demo"}')).toEqual({ host: "demo" });
  });

  it("drops a seat with a missing part, and keeps the host", () => {
    expect(parseLastOpen('{"host":"local","seat":{"id":"a-b"}}')).toEqual({ host: "local" });
  });

  it("returns null for nothing, junk, or a record with no host", () => {
    expect(parseLastOpen(null)).toBeNull();
    expect(parseLastOpen("not json")).toBeNull();
    expect(parseLastOpen('{"seat":{"id":"a","who":"b"}}')).toBeNull();
    expect(parseLastOpen('"local"')).toBeNull();
  });
});

describe("seatToRestore", () => {
  const last = { host: "local", seat: { id: "a-b", who: "Imp-Dragonfly" } };

  it("opens the remembered seat while it is listed", () => {
    expect(seatToRestore(last, [{ id: "x" }, { id: "a-b" }])).toEqual({ open: "a-b" });
  });

  it("names the seat when it is no longer listed", () => {
    expect(seatToRestore(last, [{ id: "x" }])).toEqual({ gone: "Imp-Dragonfly" });
  });

  it("has nothing to do when the host page was the last screen", () => {
    expect(seatToRestore({ host: "local" }, [{ id: "x" }])).toBeNull();
  });
});
