import { describe, expect, it } from "vitest";
import { FEEDBACK_TEXT_MAX, canSend, contextFor, contextLine, feedbackMissing, feedbackRefusal, prepareScreenshots, sendingLine, viewName } from "./feedback";

const scene = { host: "This Mac", seat: { identity: "Imp-Dragonfly", role: "frontend-eng", state: "idle" }, starting: null, place: "app", build: "1a2b3c4" };
const screen = { width: 360, height: 780, ratio: 3 };

describe("what goes along", () => {
  it("names the seat on screen, the start screen, or the host list", () => {
    expect(viewName(scene)).toBe("Seat Imp-Dragonfly (frontend-eng, idle)");
    expect(viewName({ seat: null, starting: "scientist" })).toBe("Start screen for scientist");
    expect(viewName({ seat: null, starting: null })).toBe("Host list");
  });

  it("reads as one line a person can check", () => {
    const context = contextFor(scene, screen, new Date("2026-11-04T20:00:00Z"));
    expect(context).toMatchObject({ app: true, at: "2026-11-04T20:00:00.000Z", build: "1a2b3c4" });
    expect(contextLine(context)).toBe("Seat Imp-Dragonfly (frontend-eng, idle) // 360x780 // Android app, build 1a2b3c4");
    expect(contextLine({ ...context, app: false })).toContain("browser");
  });
});

describe("what Send needs", () => {
  it("takes a note, or a screenshot alone, but not whitespace", () => {
    expect(canSend("", 0)).toBe(false);
    expect(canSend("  \n ", 0)).toBe(false);
    expect(canSend("the menu overlaps", 0)).toBe(true);
    expect(canSend("", 1)).toBe(true);
  });

  it("counts the screenshots being sent", () => {
    expect(sendingLine(0, 2)).toBe("Sending screenshot 1 of 2.");
    expect(sendingLine(2, 2)).toBe("Sending your note.");
    expect(sendingLine(0, 0)).toBe("Sending your note.");
  });
});

describe("the host's refusals", () => {
  it("words the ones this client knows and leaves the rest to the host's own text", () => {
    expect(feedbackRefusal("text_too_long")).toContain(FEEDBACK_TEXT_MAX.toLocaleString("en-US"));
    expect(feedbackRefusal("passkey_required")).toMatch(/unlocked/);
    expect(feedbackRefusal("nope")).toBeNull();
  });
});

describe("screenshots", () => {
  const png = (name: string, size = 10) => new File([new Uint8Array(size)], name, { type: "image/png" });

  it("takes images up to the limit and says why it left one out", async () => {
    const taken = await prepareScreenshots([png("a.png"), new File(["x"], "notes.txt", { type: "text/plain" })], 0, { maxBytes: 100 });
    expect(taken.ready.map((file) => file.name)).toEqual(["a.png"]);
    expect(taken.refusal).toMatch(/notes\.txt is not an image/);
  });

  it("stops at four, counting those already waiting", async () => {
    const taken = await prepareScreenshots([png("a.png"), png("b.png")], 3, { maxBytes: 100 });
    expect(taken.ready).toHaveLength(1);
    expect(taken.refusal).toMatch(/4 images at most/);
  });
});


describe("why a host does not take feedback", () => {
  const OLDER = "This host needs a newer Treehouse.";

  it("says a newer Treehouse only for a daemon older than the first that takes feedback", () => {
    for (const version of ["aos-v0.517.9", "v0.400.0", "0.9.0", "aos-v0.0.1"]) expect(feedbackMissing(version), version).toBe(OLDER);
  });

  it("does not say it for the release that added feedback, a later one, or the rig's build", () => {
    for (const version of ["aos-v0.518.0", "aos-v0.578.0", "v0.600.2", "1.0.0"]) expect(feedbackMissing(version), version).not.toBe(OLDER);
    expect(feedbackMissing("aos-v0.578.0")).toBe("This host has nowhere to keep feedback.");
  });

  it("claims nothing when the host named no build it can read", () => {
    for (const version of [undefined, "", "dev", "testbed"]) expect(feedbackMissing(version), String(version)).toBe("This host does not take feedback.");
  });

  it("words the screenshot refusals", () => {
    expect(feedbackRefusal("image_expired")).toMatch(/after ten minutes/);
    expect(feedbackRefusal("too_large")).toContain("3 MB");
  });
});
