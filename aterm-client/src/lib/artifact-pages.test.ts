import { describe, expect, it } from "vitest";
import { fromText, PAGE_HOSTS, PAGE_SANDBOX, pageDocument, pageRows, readsLiveState } from "./artifact-pages";
import type { SeatArtifact } from "./protocol";

const page = (id: string, extra: Partial<SeatArtifact> = {}): SeatArtifact => ({
  id,
  session: "role-imp",
  title: `Page ${id}`,
  url: `https://claude.ai/artifact/${id}`,
  html: "<p>x</p>",
  standing: false,
  waiting: false,
  seatEnded: false,
  ...extra,
});
const seats = [{ id: "role-imp", identity: "Imp-Dragonfly" }];

describe("the frame a page is drawn in", () => {
  it("is sandboxed to scripts alone, so a page never gets this app's origin or storage", () => {
    expect(PAGE_SANDBOX).toBe("allow-scripts");
    expect(PAGE_SANDBOX).not.toContain("allow-same-origin");
  });

  it("pins a policy first that lets only the Artifact tool's hosts load", () => {
    const doc = pageDocument("<!doctype html><html><head><title>T</title></head><body>hi</body></html>");
    expect(doc.indexOf("Content-Security-Policy")).toBeLessThan(doc.indexOf("<title>"));
    for (const host of PAGE_HOSTS) expect(doc, host).toContain(host);
    expect(doc).toContain("default-src 'none'");
    expect(doc).toContain("connect-src 'none'");
    expect(doc).toContain("form-action 'none'");
  });

  it("adds a head to a fragment, so even a bare page carries the policy", () => {
    expect(pageDocument("<p>bare</p>")).toMatch(/<head><meta http-equiv="Content-Security-Policy"/);
  });
});

describe("a page that reads live state", () => {
  it("is found by window.claude, however it is spaced", () => {
    expect(readsLiveState("<script>window.claude.use('db')</script>")).toBe(true);
    expect(readsLiveState("window . claude?.hot")).toBe(true);
    expect(readsLiveState("<p>the claude window</p>")).toBe(false);
  });
});

describe("pageRows", () => {
  it("puts waiting pages first and keeps arrival order within each", () => {
    const rows = pageRows([page("a"), page("b", { waiting: true }), page("c"), page("d", { waiting: true })], seats);
    expect(rows.map((row) => row.id)).toEqual(["b", "d", "a", "c"]);
  });

  it("names the posting seat, and falls back to its session name", () => {
    expect(pageRows([page("a")], seats)[0]?.from).toBe("Imp-Dragonfly");
    expect(pageRows([page("a", { session: "gone-seat" })], seats)[0]?.from).toBe("gone-seat");
  });

  it("does not call a page waiting once its seat has ended, and says so", () => {
    const row = pageRows([page("a", { waiting: true, seatEnded: true })], seats)[0]!;
    expect([row.waiting, row.seatEnded]).toEqual([false, true]);
    expect(fromText(row)).toBe("from Imp-Dragonfly, seat ended");
    expect(fromText({ from: "Imp-Dragonfly", seatEnded: false })).toBe("from Imp-Dragonfly");
  });

  it("marks a page that answers on claude.ai, and offers only a claude.ai artifact address", () => {
    const rows = pageRows([page("a", { html: "window.claude.use('db')" }), page("b", { url: "https://example.com/artifact/b" })], seats);
    expect(rows[0]?.answerOnPage).toBe(true);
    expect(rows[0]?.openable).toBe(true);
    expect(rows[1]?.answerOnPage).toBe(false);
    expect(rows[1]?.openable).toBe(false);
  });

  it("gives a page with no title a name, so a row is never blank", () => {
    expect(pageRows([page("a", { title: "" })], seats)[0]?.title).toBe("A page");
  });
});
