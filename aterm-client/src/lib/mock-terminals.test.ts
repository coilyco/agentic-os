import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MockHost, terminalModeFrom, type TerminalMode } from "./mock-host";
import type { HostEvent } from "./protocol";

function open(mode: TerminalMode) {
  const host = new MockHost(1200, mode);
  const events: HostEvent[] = [];
  host.subscribe((event) => events.push(event));
  return { host, events, kinds: () => events.map((event) => event.type) };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("the demo host's terminals", () => {
  it("lists none up front, so an empty list reads as none and not as unasked", () => {
    const { events } = open("ok");
    expect(events.find((event) => event.type === "terminals")).toEqual({ type: "terminals", terminals: [] });
  });

  it("says it has the feature, so the pane can tell a quiet host from one without it", () => {
    expect(open("ok").events).toContainEqual({ type: "features", terminals: true });
  });

  it("carries no channel and says so when it plays a host without the kind", () => {
    const { host, events, kinds } = open("off");
    expect(host.terminals).toBeUndefined();
    expect(events).toContainEqual({ type: "features", terminals: false });
    expect(kinds()).not.toContain("terminals");
  });

  it("never sends the first list when it plays a host that goes quiet", () => {
    expect(open("mute").kinds()).not.toContain("terminals");
  });

  it("opens a shell tagged with the seat, and takes typing as a shell does", () => {
    const { host, events } = open("ok");
    host.terminals!.open("sysadmin-senior-turtle-ox-gj84");
    vi.advanceTimersByTime(300);
    expect(events.filter((event) => event.type === "terminals").at(-1)).toEqual({ type: "terminals", terminals: [{ id: "terminal-1", label: "sysadmin-senior-turtle-ox-gj84" }] });
    expect(events.at(-1)).toEqual({ type: "terminal_opened", label: "sysadmin-senior-turtle-ox-gj84", id: "terminal-1" });
    host.attach("terminal-1");
    host.input("terminal-1", "echo ok\r");
    const out = events.filter((event) => event.type === "output").map((event) => (event.type === "output" ? String(event.data) : "")).join("");
    expect(out).toContain("echo ok");
    expect(out).toContain("\r\nok\r\n");
  });

  it("lists each shell with the seat it was opened beside, as a daemon with terminal-label does", () => {
    const { host, events } = open("ok");
    host.terminals!.open("eng-platform-beetle-ox-eb64");
    host.terminals!.open("scientist-frog-ox-va67");
    vi.advanceTimersByTime(300);
    expect(events.filter((event) => event.type === "terminals").at(-1)).toEqual({
      type: "terminals",
      terminals: [
        { id: "terminal-1", label: "eng-platform-beetle-ox-eb64" },
        { id: "terminal-2", label: "scientist-frog-ox-va67" },
      ],
    });
  });

  it("drops the shell from the list and then reports its exit, the order hardest on the pane", () => {
    const { host, events } = open("ok");
    host.terminals!.open("seat");
    vi.advanceTimersByTime(300);
    host.attach("terminal-1");
    events.length = 0;
    host.input("terminal-1", "exit 3\r");
    const frames = events.filter((event) => event.type !== "output");
    expect(frames).toEqual([
      { type: "terminals", terminals: [] },
      { type: "terminal_exit", id: "terminal-1", code: 3 },
    ]);
  });

  it("drops a closed shell from the list with no exit, the way another screen's close looks", () => {
    const { host, events } = open("ok");
    host.terminals!.open("seat");
    vi.advanceTimersByTime(300);
    events.length = 0;
    host.terminals!.close("terminal-1");
    expect(events.map((event) => event.type)).toEqual(["terminals"]);
  });

  it("keeps a seat's typing out of the shell and the shell's out of the seats", () => {
    const { host, events } = open("ok");
    host.terminals!.open("seat");
    vi.advanceTimersByTime(300);
    const sessions = events.filter((event) => event.type === "sessions").at(-1);
    expect(sessions && sessions.type === "sessions" ? sessions.sessions.some((each) => each.id.startsWith("terminal")) : true).toBe(false);
  });

  it("refuses a spawn with words, and stays silent when told to", () => {
    const refuse = open("refuse");
    refuse.host.terminals!.open("seat");
    vi.advanceTimersByTime(300);
    expect(refuse.events.at(-1)).toMatchObject({ type: "terminal_refused", label: "seat" });
    const silent = open("silent");
    silent.host.terminals!.open("seat");
    vi.advanceTimersByTime(60_000);
    expect(silent.events.some((event) => event.type === "terminal_refused")).toBe(false);
  });

  it("plays a browser the typing guard refuses", () => {
    expect(open("locked").events).toContainEqual({ type: "typing", typing: { allowed: false, reason: "peer_unread" } });
  });
});

describe("terminalModeFrom", () => {
  it("reads the mode from the page's query, and works when it is missing or wrong", () => {
    expect(terminalModeFrom("?terminals=refuse")).toBe("refuse");
    expect(terminalModeFrom("?a=1&terminals=off")).toBe("off");
    expect(terminalModeFrom("")).toBe("ok");
    expect(terminalModeFrom("?terminals=rm-rf")).toBe("ok");
  });
});
