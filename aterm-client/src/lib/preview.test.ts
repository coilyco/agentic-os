import { describe, expect, it } from "vitest";
import { previewFor } from "./preview";
import type { Ask } from "./protocol";

const ask = (question: string): Ask => ({ id: "x", session: "a", header: "Pick", question, options: [], allowOther: false, multi: false });

describe("previewFor", () => {
  it("says an asking seat is asking and shows its question on one line", () => {
    expect(previewFor({ identity: "Imp-Dragonfly", kind: "asking" }, ask("Merge   the PR\nnow?"))).toEqual({
      identity: "Imp-Dragonfly",
      kind: "asking",
      heading: "Asking you",
      detail: "Merge the PR now?",
    });
  });

  it("cuts a long question at a length a popup can hold", () => {
    const detail = previewFor({ identity: "a", kind: "asking" }, ask("word ".repeat(200)))?.detail ?? "";
    expect(detail.length).toBeLessThanOrEqual(281);
    expect(detail.endsWith("…")).toBe(true);
  });

  it("gives a finished seat only its state, since the client holds no text of its last reply", () => {
    expect(previewFor({ identity: "Frog-Ox", kind: "done" }, undefined)).toEqual({ identity: "Frog-Ox", kind: "done", heading: "Done, your turn", detail: null });
  });

  it("gives an asking seat with no ask in hand no detail rather than an empty line", () => {
    expect(previewFor({ identity: "a", kind: "asking" }, undefined).detail).toBeNull();
  });
});
