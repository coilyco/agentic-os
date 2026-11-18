// Reads Claude Code's logged-out screens off the terminal so the client can draw a
// native sign-in card (COI-3678) and route the login to the device's own browser
// with a paste-code field (COI-3679). Shapes and limits: references/login.md.

/** The login method menu. Its menu has no footer, so detectChoice never reads it. */
export interface LoginMethod {
  kind: "method";
  /** Row labels as the seat prints them, for the screen reader and the test. */
  options: string[];
  /** Where the seat's highlight is. */
  cursor: number;
  /** The "Claude account with subscription" row, the only one the card offers. */
  subscription: number;
}

/** The sign-in screen: a link, and a field that takes the code the page shows. */
export interface LoginCode {
  kind: "code";
  /** The authorize address when it is one the card may open, else null. */
  url: string | null;
}

/** A code the seat refused. Enter goes back to the method menu. */
export interface LoginError {
  kind: "error";
  message: string;
}

export type Login = LoginMethod | LoginCode | LoginError;

const METHOD_HEAD = /Select login method/;
const PASTE = /Paste code here if prompted/;
const ERROR_HEAD = /^\s*OAuth error:\s*/;
const RETRY = /Press Enter to retry/;
const OPTION = /^\s*(❯)?\s*(\d+)\.\s+(.+?)\s*$/;
// Hosts the card may open. A seat prints whatever it likes.
const AUTHORIZE_HOSTS = new Set(["claude.com", "claude.ai", "platform.claude.com", "console.anthropic.com"]);

/** The address when it is an https authorize link on a Claude host, else null. */
export function signInUrl(raw: string): string | null {
  try {
    const { protocol, hostname, port, username, password, pathname } = new URL(raw);
    const ok = protocol === "https:" && AUTHORIZE_HOSTS.has(hostname) && port === "" && username === "" && password === "" && /\/oauth\/authorize$/.test(pathname);
    return ok ? raw : null;
  } catch {
    return null;
  }
}

/** A link the seat wrapped at the terminal's width, joined with nothing between. */
function linkAbove(rows: readonly string[], before: number): string {
  const start = rows.slice(0, before).findLastIndex((row) => row.trim().startsWith("https://"));
  if (start === -1) return "";
  let end = start;
  // A link has no spaces, so the first blank or spoken row ends it.
  while (end < before && rows[end]!.trim() !== "" && !/\s/.test(rows[end]!.trim())) end++;
  return rows
    .slice(start, end)
    .map((row) => row.trim())
    .join("");
}

function readError(rows: readonly string[]): LoginError | null {
  const at = rows.findLastIndex((row) => ERROR_HEAD.test(row));
  if (at === -1) return null;
  // The message wraps onto more rows the narrower the screen. The retry line ends it.
  const retry = rows.findIndex((row, index) => index > at && RETRY.test(row));
  if (retry === -1 || retry - at > 12) return null;
  const message = rows
    .slice(at, retry)
    .map((row) => row.trim())
    .filter((row) => row !== "")
    .join(" ")
    .replace(ERROR_HEAD, "");
  return { kind: "error", message };
}

function readMethod(rows: readonly string[]): LoginMethod | null {
  const head = rows.findLastIndex((row) => METHOD_HEAD.test(row));
  if (head === -1) return null;
  const options: string[] = [];
  let cursor = -1;
  for (const row of rows.slice(head + 1)) {
    const option = OPTION.exec(row);
    if (option) {
      if (option[1]) cursor = options.length;
      options.push(option[3]!);
    } else if (options.length > 0) {
      // A narrow terminal wraps a long option onto the next row. A blank row ends it.
      if (row.trim() === "") break;
      options[options.length - 1] += ` ${row.trim()}`;
    }
  }
  const subscription = options.findIndex((label) => /subscription/i.test(label));
  return options.length < 2 || cursor === -1 || subscription === -1 ? null : { kind: "method", options, cursor, subscription };
}

/** The logged-out screen showing, or null. Error outranks link, which outranks menu. */
export function detectLogin(screen: readonly string[]): Login | null {
  const rows = screen.map((row) => row.replace(/\s+$/, ""));
  const error = readError(rows);
  if (error) return error;
  const paste = rows.findLastIndex((row) => PASTE.test(row));
  if (paste !== -1) return { kind: "code", url: signInUrl(linkAbove(rows, paste)) };
  return readMethod(rows);
}

/** What makes two readings one card: kind and link, never the field's contents. */
export function loginKey(login: Login): string {
  return login.kind === "code" ? `code ${login.url ?? ""}` : login.kind === "error" ? `error ${login.message}` : "method";
}

const DOWN = "\x1b[B";
const UP = "\x1b[A";

/** Arrows from the highlight to the subscription row, then Enter. */
export function methodKeys(login: LoginMethod): string[] {
  const steps = login.subscription - login.cursor;
  return [...(steps === 0 ? [] : [(steps > 0 ? DOWN : UP).repeat(Math.abs(steps))]), "\r"];
}

/** Exactly the pasted text, then Enter. Only edge whitespace goes (keyboards add it). */
export function codeKeys(pasted: string): string[] {
  const code = pasted.trim();
  return code === "" ? [] : [code, "\r"];
}

/** Enter on the error screen goes back to the method menu. */
export const RETRY_KEYS = ["\r"];
/** Esc on the error screen continues without logging in. */
export const CONTINUE_KEYS = ["\x1b"];
