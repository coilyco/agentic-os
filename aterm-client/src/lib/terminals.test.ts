import { describe, expect, it } from "vitest";
import { TYPING_OPEN } from "./typing";
import { applyList, loadClosed, paneState, paneText, recordExit, shouldOpen, storeClosed, terminalFor, type PaneInput, type TerminalEntry } from "./terminals";

const live = (id: string, label: string | null = "sysadmin-senior-turtle-ox"): TerminalEntry => ({ id, label, exit: null });

function input(over: Partial<PaneInput> = {}): PaneInput {
  return { supported: true, loaded: true, typing: TYPING_OPEN, entry: undefined, opening: false, refusal: undefined, closed: false, ...over };
}

describe("paneState, worst first", () => {
  it("says the host does not carry terminals before anything else", () => {
    const state = paneState(input({ supported: false, entry: live("t1"), opening: true, refusal: "no" }));
    expect(state.kind).toBe("unsupported");
  });

  it("waits for the first list rather than opening a second shell on a reload", () => {
    const state = paneState(input({ loaded: false }));
    expect(state.kind).toBe("waiting");
    expect(shouldOpen(state)).toBe(false);
  });

  it("shows an existing shell even when typing is refused, so it can still be watched", () => {
    const entry = live("t1");
    expect(paneState(input({ entry, typing: { allowed: false, reason: "peer_unread" } }))).toEqual({ kind: "live", entry });
  });

  it("names the exit code of a shell that ended", () => {
    const entry: TerminalEntry = { ...live("t1"), exit: { code: 2 } };
    expect(paneState(input({ entry }))).toEqual({ kind: "exited", entry, code: 2 });
  });

  it("reports a refused spawn, and does not retry on its own", () => {
    const state = paneState(input({ refusal: "no shell on this host" }));
    expect(state).toEqual({ kind: "refused", text: "no shell on this host" });
    expect(shouldOpen(state)).toBe(false);
  });

  it("does not offer to open when the typing guard refuses this browser", () => {
    const state = paneState(input({ typing: { allowed: false, reason: "session_descendant" } }));
    expect(state).toEqual({ kind: "locked", reason: "session_descendant" });
    expect(shouldOpen(state)).toBe(false);
  });

  it("does not reopen a terminal Kai closed", () => {
    const state = paneState(input({ closed: true }));
    expect(state.kind).toBe("closed");
    expect(shouldOpen(state)).toBe(false);
  });

  it("opens one on its own only when everything allows it", () => {
    const state = paneState(input());
    expect(state.kind).toBe("ready");
    expect(shouldOpen(state)).toBe(true);
  });

  it("shows nothing to open while a spawn is in flight", () => {
    const state = paneState(input({ opening: true }));
    expect(state.kind).toBe("opening");
    expect(shouldOpen(state)).toBe(false);
  });
});

describe("applyList", () => {
  it("takes the daemon's list as the truth for live shells", () => {
    expect(applyList([live("t1")], [{ id: "t2", label: "b" }])).toEqual([live("t2", "b")]);
  });

  it("keeps a shell that exited after the daemon drops it, so the pane can say so", () => {
    const exited = recordExit([live("t1")], "t1", 130);
    expect(applyList(exited, [])).toEqual([{ ...live("t1"), exit: { code: 130 } }]);
  });

  it("drops the exited shell once a new one opens for the same seat", () => {
    const exited = recordExit([live("t1")], "t1", 0);
    expect(applyList(exited, [{ id: "t2", label: "sysadmin-senior-turtle-ox" }])).toEqual([live("t2")]);
  });

  it("drops a shell that vanished without an exit, as when it was closed from another screen", () => {
    expect(applyList([live("t1")], [])).toEqual([]);
  });

  it("ignores an exit for a terminal it never heard of", () => {
    expect(recordExit([live("t1")], "ghost", 1)).toEqual([live("t1")]);
  });
});

describe("terminalFor", () => {
  it("finds the terminal beside a seat, preferring a live one over an exited one", () => {
    const exited: TerminalEntry = { id: "t1", label: "a", exit: { code: 0 } };
    expect(terminalFor([exited, live("t2", "a"), live("t3", "b")], "a")?.id).toBe("t2");
    expect(terminalFor([exited], "a")?.id).toBe("t1");
    expect(terminalFor([live("t3", "b")], "a")).toBeUndefined();
  });

  it("finds nothing for a terminal the daemon did not tag", () => {
    expect(terminalFor([live("t1", null)], "a")).toBeUndefined();
  });
});

describe("paneText", () => {
  it("has words for every state that has no shell, and none for the ones that do", () => {
    const entry = live("t1");
    const states = [
      { kind: "unsupported" },
      { kind: "waiting" },
      { kind: "opening" },
      { kind: "refused", text: "x" },
      { kind: "locked", reason: "peer_unread" },
      { kind: "closed" },
      { kind: "ready" },
    ] as const;
    for (const state of states) expect(paneText(state, "Turtle-Ox")?.title).toBeTruthy();
    expect(paneText({ kind: "live", entry }, "Turtle-Ox")).toBeNull();
    expect(paneText({ kind: "exited", entry, code: 0 }, "Turtle-Ox")).toBeNull();
  });

  it("names the seat and the reason, so the empty state says whose shell and why not", () => {
    expect(paneText({ kind: "ready" }, "Turtle-Ox")?.title).toContain("Turtle-Ox");
    expect(paneText({ kind: "locked", reason: "peer_unread" }, "x")?.body).toContain("peer_unread");
    expect(paneText({ kind: "refused", text: "no shell here" }, "x")?.body).toBe("no shell here");
  });
});

describe("closed seats", () => {
  it("survive a reload, and a broken store reads as none", () => {
    let held = "";
    storeClosed(["a", "b"], { setItem: (_key, value) => void (held = value) });
    expect(loadClosed({ getItem: () => held })).toEqual(["a", "b"]);
    expect(loadClosed({ getItem: () => "{oops" })).toEqual([]);
    expect(loadClosed({ getItem: () => { throw new Error("blocked"); } })).toEqual([]);
    expect(() => storeClosed(["a"], { setItem: () => { throw new Error("full"); } })).not.toThrow();
  });

  it("keeps only the most recent, so the store does not grow with every seat ever opened", () => {
    let held = "";
    storeClosed(Array.from({ length: 200 }, (_, index) => `s${index}`), { setItem: (_key, value) => void (held = value) });
    const kept = loadClosed({ getItem: () => held });
    expect(kept).toHaveLength(64);
    expect(kept.at(-1)).toBe("s199");
  });
});
