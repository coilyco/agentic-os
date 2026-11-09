import { describe, expect, it } from "vitest";
import { noticeFor, SeenIds, wantsDesktopNotice } from "./notify";
import type { Notify } from "./protocol";

const note: Notify = { id: "n1", session: "prod-director-griffin-goose", role: "prod-director", identity: "Griffin-Goose", title: "Decision needed", body: "Pick the order", at: 1 };

describe("SeenIds", () => {
  it("is true once per id", () => {
    const seen = new SeenIds();
    expect(seen.first("a")).toBe(true);
    expect(seen.first("a")).toBe(false);
    expect(seen.first("b")).toBe(true);
  });
  it("forgets the oldest past its limit", () => {
    const seen = new SeenIds();
    for (let i = 0; i < 70; i++) seen.first(`id-${i}`);
    expect(seen.first("id-0")).toBe(true);
    expect(seen.first("id-69")).toBe(false);
  });
});

describe("wantsDesktopNotice", () => {
  it("skips a pop-up when the person is on the pinging seat", () => {
    expect(wantsDesktopNotice({ focused: true, openSeat: note.session }, note)).toBe(false);
  });
  it("shows it from another seat, or when the page is not focused", () => {
    expect(wantsDesktopNotice({ focused: true, openSeat: "other" }, note)).toBe(true);
    expect(wantsDesktopNotice({ focused: false, openSeat: note.session }, note)).toBe(true);
  });
});

describe("noticeFor", () => {
  it("uses the seat's words, one tag per seat, and no sound of its own", () => {
    const notice = noticeFor(note);
    expect(notice.title).toBe("Decision needed");
    expect(notice.options).toEqual({ body: "Pick the order", tag: "aterm-prod-director-griffin-goose", silent: true, data: { session: note.session } });
  });
});
