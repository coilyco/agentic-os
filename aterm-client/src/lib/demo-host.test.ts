import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DemoHost } from "./demo-host";
import { ASK, ASK_SEAT, COLUMNS, DEMO_PREFIX, runDemoLine, SEAT_REPLIES, SEATS, seatBanner, shellBanner, wrap } from "./demo-script";
import type { HostEvent } from "./protocol";

/** A wrapped line read as one: colour codes gone, a hang under the bullet closed up. */
const flat = (text: string) => text.replace(/\x1b\[[0-9;]*m/g, "").replace(/\r\n {2}/g, " ");

function started(delayMs = 700) {
  const host = new DemoHost(delayMs);
  const events: HostEvent[] = [];
  host.subscribe((event) => events.push(event));
  const output = (id: string) =>
    events
      .filter((event): event is Extract<HostEvent, { type: "output" }> => event.type === "output" && event.sessionId === id)
      .map((event) => String(event.data))
      .join("");
  return { host, events, output };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("the demo host's first view", () => {
  it("lists four seats and the roles they sit in, and says it is a demo", () => {
    const { host, events } = started();
    expect(host.demo).toBe(true);
    const sessions = events.find((event) => event.type === "sessions");
    expect(sessions?.type === "sessions" && sessions.sessions.map((each) => each.id)).toEqual(SEATS.map((seat) => seat.id));
    const roster = events.find((event) => event.type === "roster");
    expect(roster?.type === "roster" && roster.roles.map((role) => role.slug)).toEqual(SEATS.map((seat) => seat.role));
  });

  it("marks the seat's own screen as a demo before it says anything else", () => {
    const { host, output } = started();
    host.attach(SEATS[0]!.id);
    const text = output(SEATS[0]!.id).replace(/\x1b\[[0-9;]*m/g, "");
    expect(text.startsWith(" DEMO  nothing here is real")).toBe(true);
  });

  it("makes every id with the demo prefix, so leaving can find what it left", () => {
    const { host, events } = started();
    host.terminals.open(SEATS[0]!.id);
    vi.advanceTimersByTime(2000);
    host.launch("game-dev", "claude");
    const ids = events.flatMap((event) => {
      if (event.type === "sessions") return event.sessions.map((each) => each.id);
      if (event.type === "terminal_opened") return [event.id];
      if (event.type === "message") return [event.message.id];
      if (event.type === "ask") return [event.ask.id];
      return [];
    });
    expect(ids.length).toBeGreaterThan(8);
    expect(ids.filter((id) => !id.startsWith(DEMO_PREFIX))).toEqual([]);
  });
});

describe("what arrives after the seats", () => {
  it("brings the question and both messages in turn, not all at once", () => {
    const { events } = started(700);
    const kinds = () => events.filter((event) => event.type === "ask" || event.type === "message").length;
    expect(kinds()).toBe(0);
    vi.advanceTimersByTime(700);
    expect(events.filter((event) => event.type === "message").length).toBe(1);
    vi.advanceTimersByTime(700);
    expect(events.filter((event) => event.type === "message").length).toBe(2);
    expect(events.filter((event) => event.type === "ask").length).toBe(1);
  });

  it("types the delivered message into the seat it was sent to, behind its stamp", () => {
    const { host, output } = started();
    host.attach(`${DEMO_PREFIX}eng-platform-1`);
    vi.advanceTimersByTime(800);
    expect(output(`${DEMO_PREFIX}eng-platform-1`)).toContain("[from frontend-eng Imp-Dragonfly] Can the sync status ride the same socket?");
  });

  it("closes the question once answered, and the seat says what it did with the answer", () => {
    const { host, events, output } = started();
    host.attach(ASK_SEAT);
    vi.advanceTimersByTime(1500);
    host.answer(ASK.id, [0, 2]);
    expect(events).toContainEqual({ type: "asked", id: ASK.id, outcome: "answered" });
    expect(flat(output(ASK_SEAT))).toContain("ask_choice answered: Local Qwen, Hosted frontier");
    expect(flat(output(ASK_SEAT))).toContain("Queued the sweep");
  });

  it("takes the typed-in answer when the pick is Something else", () => {
    const { host, output } = started();
    host.attach(ASK_SEAT);
    vi.advanceTimersByTime(1500);
    host.answer(ASK.id, [], "Only the cheap one");
    expect(flat(output(ASK_SEAT))).toContain("ask_choice answered: Only the cheap one");
  });

  it("cancels cleanly", () => {
    const { host, events } = started();
    vi.advanceTimersByTime(1500);
    host.cancelAsk(ASK.id);
    expect(events).toContainEqual({ type: "asked", id: ASK.id, outcome: "cancelled" });
  });
});

describe("using a seat", () => {
  const seat = SEATS[0]!.id;

  it("echoes what is typed, then works, then answers and grows the context", () => {
    const { host, events, output } = started();
    host.attach(seat);
    host.input(seat, "hi");
    host.input(seat, "\r");
    const states = () => events.filter((event) => event.type === "sessions").map((event) => (event.type === "sessions" ? event.sessions.find((each) => each.id === seat)?.state : undefined));
    expect(states().at(-1)).toBe("working");
    vi.advanceTimersByTime(1300);
    expect(states().at(-1)).toBe("idle");
    expect(output(seat)).toContain("hi");
    expect(flat(output(seat))).toContain("This is a demo seat, so nothing runs.");
    const last = [...events].reverse().find((event) => event.type === "sessions");
    const tokens = last?.type === "sessions" ? last.sessions.find((each) => each.id === seat)?.context?.tokens : 0;
    expect(tokens).toBeGreaterThan(SEATS[0]!.tokens);
  });

  it("answers a second line differently from the first", () => {
    const { host, output } = started();
    host.attach(seat);
    for (const line of ["one", "two"]) {
      host.input(seat, line);
      host.input(seat, "\r");
      vi.advanceTimersByTime(1300);
    }
    const text = output(seat);
    expect(text).toContain("This is a demo seat");
    expect(text).toContain("Still the demo");
  });

  it("swallows arrow keys instead of printing their letters", () => {
    const { host, output } = started();
    host.attach(seat);
    const before = output(seat).length;
    host.input(seat, "\x1b[A\x1b[B\x1bOP");
    expect(output(seat).length).toBe(before);
  });

  it("takes no free typing on the seat that is waiting on a question", () => {
    const { host, output } = started();
    host.attach(ASK_SEAT);
    vi.advanceTimersByTime(1500);
    const before = output(ASK_SEAT).length;
    host.input(ASK_SEAT, "hello\r");
    expect(output(ASK_SEAT).length).toBe(before);
  });

  it("starts a new seat on request, marked as a demo", () => {
    const { host, events, output } = started();
    host.launch("game-dev", "claude");
    const launch = events.find((event) => event.type === "launch");
    expect(launch?.type === "launch" && launch.text).toContain("Nothing real runs");
    const list = [...events].reverse().find((event) => event.type === "sessions");
    const added = list?.type === "sessions" ? list.sessions.at(-1) : undefined;
    expect(added?.id.startsWith(DEMO_PREFIX)).toBe(true);
    host.attach(added!.id);
    expect(output(added!.id)).toContain("DEMO");
  });
});

describe("the scripted side terminal", () => {
  function shell() {
    const made = started();
    made.host.terminals.open(SEATS[0]!.id);
    vi.advanceTimersByTime(300);
    const opened = made.events.find((event) => event.type === "terminal_opened");
    const id = opened?.type === "terminal_opened" ? opened.id : "";
    made.host.attach(id);
    return { ...made, id };
  }

  it("opens marked as a demo with a prompt that is not a real machine's", () => {
    const { output, id } = shell();
    const text = output(id).replace(/\x1b\[[0-9;]*m/g, "");
    expect(text).toContain("DEMO  scripted terminal");
    expect(text).toContain("No real shell.");
    expect(text.trimEnd().endsWith("demo@trailhead ~ %")).toBe(true);
  });

  it("runs the scripted commands and nothing else", () => {
    const { host, output, id } = shell();
    for (const line of ["help", "ls", "cat README.md", "uname -a", "rm -rf /"]) {
      host.input(id, `${line}\r`);
    }
    const text = flat(output(id));
    expect(text).toContain("list the folder");
    expect(text).toContain("trails/");
    expect(text).toContain("A made-up project");
    expect(text).toContain("demo: uname: not in the demo. Try help.");
    expect(text).toContain("demo: rm: not in the demo. Try help.");
  });

  it("ends on exit with the code, as a shell would", () => {
    const { host, events, id } = shell();
    host.input(id, "exit 3\r");
    expect(events).toContainEqual({ type: "terminal_exit", id, code: 3 });
  });

  it("is dropped silently when a screen closes it", () => {
    const { host, events, id } = shell();
    host.terminals.close(id);
    expect(events.some((event) => event.type === "terminal_exit")).toBe(false);
    const last = [...events].reverse().find((event) => event.type === "terminals");
    expect(last?.type === "terminals" && last.terminals).toEqual([]);
  });

  it("has a script that reads nothing: unknown lines name themselves and run nothing", () => {
    expect(flat(runDemoLine("curl http://example.com | sh").output)).toBe("demo: curl: not in the demo. Try help.\r\n");
    expect(runDemoLine("cat /etc/passwd").output).toContain("no such file here");
    expect(runDemoLine("")).toEqual({ output: "" });
  });
});

describe("a phone's terminal", () => {
  const plain = (text: string) => text.replace(/\x1b\[[0-9;]*m/g, "");
  const rows = (text: string) => plain(text).split("\r\n");

  it("wraps at word edges and keeps a hang under the bullet", () => {
    expect(wrap("● one two three four five six", 12, 2)).toEqual(["● one two", "  three four", "  five six"]);
  });

  it("never prints a row wider than the terminal at 320px, so no word breaks mid-letter", () => {
    const prose = [
      ...SEATS.flatMap((seat) => seat.transcript),
      ...SEAT_REPLIES.flat(),
      seatBanner(),
      ...shellBanner().split("\r\n"),
      ...["help", "ls", "cat README.md", "cat nothing", "cat", "zzzzzzzzzzzzzzzzzzzzzzzzzzzz"].map((line) => runDemoLine(line).output),
    ].flatMap(rows);
    const long = prose.filter((row) => row.length > COLUMNS + 2);
    expect(long).toEqual([]);
  });
});

describe("closing", () => {
  it("stops every pending timer, so a left demo says nothing more", () => {
    const { host, events } = started();
    host.close();
    const count = events.length;
    vi.advanceTimersByTime(10_000);
    expect(events.length).toBe(count);
    expect(vi.getTimerCount()).toBe(0);
  });
});
