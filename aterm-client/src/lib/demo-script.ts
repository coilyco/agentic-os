// What the bundled demo says and shows (COI-3315). Every line is made up for the
// demo. Nothing here names a real host, person, or project, and nothing runs.
// Why it is its own file, apart from mock-host.ts:
// .agents/skills/tooling-aterm-client/references/demo-mode.md
import type { ShellStep } from "./demo-shell";
import type { Ask, PeerMessage } from "./protocol";

/** Every id the demo makes starts with this, so leaving can drop what it left. */
export const DEMO_PREFIX = "demo-";

const DIM = "\x1b[2m";
const RESET = "\x1b[0m";
// Reverse video in amber, the same colour as the page's own demo banner.
const TAG = "\x1b[7;38;5;215m DEMO \x1b[0m";

/** A phone terminal is about 36 columns and breaks mid-word, so prose wraps first. */
export const COLUMNS = 34;

/** Word-wraps `text`, later rows indented by `hang`. */
export function wrap(text: string, width = COLUMNS, hang = 2): string[] {
  const rows: string[] = [];
  let row = "";
  for (const word of text.split(" ")) {
    const limit = rows.length === 0 ? width : width - hang;
    if (row && row.length + 1 + word.length > limit) {
      rows.push(row);
      row = word;
    } else row = row ? `${row} ${word}` : word;
  }
  rows.push(row);
  return rows.map((each, at) => (at === 0 ? each : " ".repeat(hang) + each));
}

/** One line a seat says, as a bullet that wraps under its own text. */
export const say = (text: string): string[] => wrap(`● ${text}`);

/** What a seat prints before it says anything: the mark that none of it is real. */
export const seatBanner = (): string => `${TAG} ${DIM}nothing here is real${RESET}\r\n\r\n`;

export const SEAT_PROMPT = "\r\n› ";
export const SHELL_PROMPT = "demo@trailhead ~ % ";

export const shellBanner = (): string => `${TAG} ${DIM}scripted terminal${RESET}\r\n${DIM}No real shell. Try ${RESET}help${DIM}.${RESET}\r\n${SHELL_PROMPT}`;

/** The four seats, each a role the roster already has. Lines stay short for a phone. */
export interface DemoSeat {
  id: string;
  role: string;
  seat: string;
  state: "idle" | "working";
  tokens: number;
  pending: number;
  transcript: string[];
}

export const SEATS: DemoSeat[] = [
  {
    id: `${DEMO_PREFIX}frontend-eng-1`,
    role: "frontend-eng",
    seat: "claude",
    state: "idle",
    tokens: 61_400,
    pending: 0,
    transcript: [
      "› The trail list is a blank page",
      "  when there are no trails. Fix it.",
      "",
      `${DIM}● Read(src/TrailList.svelte)${RESET}`,
      `${DIM}  ⎿ Read 96 lines${RESET}`,
      "",
      ...say("It renders nothing for an empty list. I'll add an empty state that says what to do next."),
      "",
      `${DIM}● Update(src/TrailList.svelte)${RESET}`,
      `${DIM}  ⎿ +9 -1${RESET}`,
      `${DIM}● Bash(npm test)${RESET}`,
      `${DIM}  ⎿ 24 passed${RESET}`,
      "",
      ...say('Done. The empty list now reads "No trails yet. Add one to start."'),
    ],
  },
  {
    id: `${DEMO_PREFIX}eng-platform-1`,
    role: "eng-platform",
    seat: "claude",
    state: "working",
    tokens: 118_900,
    pending: 0,
    transcript: [
      "› Retry the sync worker when the",
      "  upload fails, with a backoff.",
      "",
      `${DIM}● Read(sync/worker.go)${RESET}`,
      `${DIM}  ⎿ Read 212 lines${RESET}`,
      `${DIM}● Update(sync/worker.go)${RESET}`,
      `${DIM}  ⎿ +42 -6${RESET}`,
      "",
      `${DIM}● Bash(go test ./sync/...)${RESET}`,
      `${DIM}  ⎿ running, 3 of 8 packages${RESET}`,
    ],
  },
  {
    id: `${DEMO_PREFIX}scientist-1`,
    role: "scientist",
    seat: "claude",
    state: "working",
    tokens: 9_300,
    pending: 1,
    transcript: [
      "› Compare the routes for latency.",
      "",
      `${DIM}● Planning the sweep${RESET}`,
      ...say("Three routes could go first. I need your pick before I spend any runs."),
    ],
  },
  {
    id: `${DEMO_PREFIX}game-dev-1`,
    role: "game-dev",
    seat: "claude",
    state: "idle",
    tokens: 22_800,
    pending: 0,
    transcript: [
      "› Make the jump feel floaty, not",
      "  stiff. Keep it playable.",
      "",
      `${DIM}● Read(game/player.gd)${RESET}`,
      `${DIM}  ⎿ Read 140 lines${RESET}`,
      `${DIM}● Update(game/player.gd)${RESET}`,
      `${DIM}  ⎿ gravity 32 → 22, +hang time${RESET}`,
      "",
      ...say("Jump height is the same. The top of the arc now lingers 80 ms."),
    ],
  },
];

/** Roles the demo roster keeps, in the order the Start a seat list shows them. */
export const DEMO_ROLES = SEATS.map((seat) => seat.role);

export const ASK_SEAT = `${DEMO_PREFIX}scientist-1`;

export const ASK: Ask = {
  id: `${DEMO_PREFIX}ask-routes`,
  session: ASK_SEAT,
  header: "Sweep",
  question: "Which routes should the latency sweep cover first?",
  options: [
    { label: "Local Qwen", description: "Fastest to turn around." },
    { label: "Local Llama", description: "The current default." },
    { label: "Hosted frontier", description: "Costs money per run." },
  ],
  allowOther: true,
  multi: true,
};

/** Seat to seat: one delivered, one waiting. Names come from the roster. */
export const messagesFor = (nameOf: (role: string) => string): { after: number; message: PeerMessage; typed?: string }[] => [
  {
    after: 1,
    message: {
      id: `${DEMO_PREFIX}m-1`,
      from: { role: "frontend-eng", identity: nameOf("frontend-eng") },
      target: "eng-platform",
      session: `${DEMO_PREFIX}eng-platform-1`,
      state: "delivered",
      reason: null,
    },
    typed: "Can the sync status ride the same socket?",
  },
  {
    after: 2,
    message: {
      id: `${DEMO_PREFIX}m-2`,
      from: { role: "eng-platform", identity: nameOf("eng-platform") },
      target: "frontend-eng",
      session: `${DEMO_PREFIX}frontend-eng-1`,
      state: "queued",
      reason: `${nameOf("frontend-eng")} is mid-turn`,
    },
  },
];

/** Replies to anything typed into a seat, in turn, so a second try says something new. */
export const SEAT_REPLIES = [
  say("This is a demo seat, so nothing runs. A real one would start on that now."),
  say("Still the demo. Real seats read files, run tests, and ask you when they need a decision."),
  say("Try the Terminal tab for a scripted shell, or answer the question on the Applied Scientist seat."),
];

/** What the scientist says once the question is answered. */
export const afterAnswer = (labels: string[]): string[] => [
  ...wrap(`● ask_choice answered: ${labels.join(", ")}`).map((row) => `${DIM}${row}${RESET}`),
  ...say("Queued the sweep on the first route. Results land here."),
];

export const afterCancel = (): string[] => [`${DIM}● ask_choice cancelled${RESET}`, ...say("No sweep started.")];

const FILES = ["README.md", "notes.txt", "trails/"];

const HELP = [
  "help      this list",
  "ls        list the folder",
  "cat FILE  print a file",
  "echo ...  print what you type",
  "pwd       print the folder",
  "whoami    print the user",
  "clear     clear the screen",
  "exit      end the terminal",
];

const CONTENTS: Record<string, string[]> = {
  "README.md": ["Trailhead demo", ...wrap("A made-up project, so this terminal has something to list.", COLUMNS, 0), ...wrap("Nothing here is on a real machine.", COLUMNS, 0)],
  "notes.txt": ["- empty state for the trail list", "- retry the sync worker", "- tune the jump arc"],
};

const lines = (rows: string[]): string => rows.map((row) => `${row}\r\n`).join("");

/** One entered line of the scripted shell. It reads nothing and runs nothing. */
export function runDemoLine(line: string): ShellStep {
  const [command = "", ...rest] = line.trim().split(/\s+/);
  switch (command) {
    case "":
      return { output: "" };
    case "help":
      return { output: lines(HELP) };
    case "ls":
      return { output: lines(FILES) };
    case "cat": {
      const body = CONTENTS[rest[0] ?? ""];
      if (body) return { output: lines(body) };
      return { output: rest[0] ? `cat: ${rest[0]}: no such file here\r\n` : "cat: name a file, e.g. cat README.md\r\n" };
    }
    case "echo":
      return { output: `${rest.join(" ")}\r\n` };
    case "pwd":
      return { output: "/home/demo\r\n" };
    case "whoami":
      return { output: "demo\r\n" };
    case "clear":
      return { output: "\x1b[2J\x1b[H" };
    case "exit": {
      const code = Number(rest[0] ?? 0);
      return { output: "logout\r\n", exit: Number.isInteger(code) ? code : 0 };
    }
    default:
      return { output: `${wrap(`demo: ${command}: not in the demo. Try help.`, COLUMNS, 2).join("\r\n")}\r\n` };
  }
}
