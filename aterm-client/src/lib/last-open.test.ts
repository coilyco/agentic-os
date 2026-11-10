import { describe, expect, it } from "vitest";
import { parseLastOpen } from "./last-open";

describe("parseLastOpen", () => {
  it("reads a host", () => {
    expect(parseLastOpen('{"host":"demo"}')).toEqual({ host: "demo" });
  });

  it("drops a seat an older build stored, so a launch rests on the overview", () => {
    expect(parseLastOpen('{"host":"local","seat":{"id":"a-b","who":"Imp-Dragonfly"}}')).toEqual({ host: "local" });
  });

  it("returns null for nothing, junk, or a record with no host", () => {
    expect(parseLastOpen(null)).toBeNull();
    expect(parseLastOpen("not json")).toBeNull();
    expect(parseLastOpen('{"seat":{"id":"a","who":"b"}}')).toBeNull();
    expect(parseLastOpen('"local"')).toBeNull();
  });
});
