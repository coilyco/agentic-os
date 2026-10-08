import { describe, expect, it } from "vitest";
import { TYPING_OPEN, typingNotice } from "./typing";
import { applyList, claim, exitText, loadClosed, loadSeats, paneState, paneText, recordExit, shouldOpen, storeClosed, storeSeats, terminalFor, type PaneInput, type TerminalEntry } from "./terminals";

const live = (id: string, label: string | null = "sysadmin-senior-turtle-ox"): TerminalEntry => ({ id, label, exit: null });

function input(over: Partial<PaneInput> = {}): PaneInput {
  return { support: "yes", loaded: true, typing: TYPING_OPEN, entry: undefined, opening: false, refusal: undefined, closed: false, ...over };
}

describe("paneState, worst first", () => {
  it("says the host does not carry terminals before anything else", () => {
    const state = paneState(input({ support: "no", entry: live("t1"), opening: true, refusal: "no" }));
    expect(state.kind).toBe("unsupported");
  });

  it("does not claim a host lacks terminals before its welcome has said", () => {
    const state = paneState(input({ support: "unknown" }));
    expect(state.kind).toBe("waiting");
    expect(shouldOpen(state)).toBe(false);
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
  const seats = { t1: "a", t2: "b" };

  it("labels each shell with the seat this browser opened it beside", () => {
    expect(applyList([], [{ id: "t1" }, { id: "t9" }], seats)).toEqual([live("t1", "a"), live("t9", null)]);
  });

  it("keeps a shell that exited before the list dropped it, with its code", () => {
    const exited = recordExit([live("t1", "a")], "t1", 130);
    expect(applyList(exited, [], seats)).toEqual([{ ...live("t1", "a"), exit: { code: 130 } }]);
  });

  it("keeps a shell the list dropped before its exit came, as ended and not as gone", () => {
    const dropped = applyList([live("t1", "a")], [], seats);
    expect(dropped).toEqual([{ ...live("t1", "a"), exit: { code: null } }]);
    expect(recordExit(dropped, "t1", 2)).toEqual([{ ...live("t1", "a"), exit: { code: 2 } }]);
  });

  it("drops the ended shell once a new one is live for the same seat", () => {
    const ended = recordExit([live("t1", "a")], "t1", 0);
    expect(applyList(ended, [{ id: "t5" }], { ...seats, t5: "a" })).toEqual([live("t5", "a")]);
  });

  it("lets a shell Kai closed leave without trace", () => {
    expect(applyList([live("t1", "a")], [], seats, new Set(["t1"]))).toEqual([]);
  });

  it("ignores an exit for a terminal it never heard of", () => {
    expect(recordExit([live("t1", "a")], "ghost", 1)).toEqual([live("t1", "a")]);
  });

  it("labels a shell once the daemon says which one it opened, even after the list showed it", () => {
    expect(claim(applyList([], [{ id: "t7" }], {}), "t7", "a")).toEqual([live("t7", "a")]);
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
    expect(paneText({ kind: "locked", reason: "peer_unread" }, "x")?.body).toBe(typingNotice({ allowed: false, reason: "peer_unread" }));
    expect(paneText({ kind: "refused", text: "no shell here" }, "x")?.body).toBe("no shell here");
  });
});

describe("exitText", () => {
  it("names the code when it is known, and says only that it ended when it is not", () => {
    expect(exitText(2)).toBe("The shell exited with code 2.");
    expect(exitText(0)).toBe("The shell exited with code 0.");
    expect(exitText(null)).toBe("The shell ended.");
  });
});

describe("seats beside terminals", () => {
  it("survive a reload, and a broken store reads as none", () => {
    let held = "";
    storeSeats({ "terminal-1": "seat-a" }, { setItem: (_key, value) => void (held = value) });
    expect(loadSeats({ getItem: () => held })).toEqual({ "terminal-1": "seat-a" });
    for (const raw of ["{oops", "[]", "7", '{"a":3}']) expect(loadSeats({ getItem: () => raw })).toEqual({});
    expect(() => storeSeats({}, { setItem: () => { throw new Error("full"); } })).not.toThrow();
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
