import { describe, expect, it } from "vitest";
import { resumeNotice, resumeProof } from "./resume";

describe("resumeProof", () => {
  it("reads a proof the daemon sent, and nothing from an absent or empty one", () => {
    expect(resumeProof({ resume: "abc" })).toBe("abc");
    expect(resumeProof({})).toBeUndefined();
    expect(resumeProof({ resume: "" })).toBeUndefined();
    expect(resumeProof({ resume: 7 })).toBeUndefined();
  });
});

describe("resumeNotice", () => {
  it("says what is happening while the proof is out, and what to do once it ended", () => {
    expect(resumeNotice("resuming")).toBe("Checking that your last unlock still holds.");
    expect(resumeNotice("ended")).toMatch(/ended while this device was away\. Unlock again to type\./);
  });
});
