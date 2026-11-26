import { describe, expect, it } from "vitest";
import { detectChoice } from "./choices";
import { loginFrame } from "./mock-harness";
import { CONTINUE_KEYS, codeKeys, detectLogin, loginKey, methodKeys, RETRY_KEYS, signInUrl, type LoginMethod } from "./login";

const X = (count: number): string => "x".repeat(count);

// Captured from Claude Code v2.1.327 started with no login, at 134 columns (COI-3679).
// One-use state and challenge are blanked one character for one, so row lengths hold.
const method = [
  "Welcome to Claude Code v2.1.327",
  "..........................................................",
  "",
  "       █████████                                        *",
  "      ██▄█████▄██                        *",
  "       █████████      *",
  ".......█ █   █ █..........................................",
  "",
  " Claude Code can be used with your Claude subscription or billed based on API usage through your Console account.",
  "",
  " Select login method:",
  "",
  " ❯ 1. Claude account with subscription · Individual, Team, or Enterprise",
  "   2. Anthropic Console account · API usage billing",
  "   3. 3rd-party platform · Bedrock, Microsoft Foundry, Google Vertex AI",
  "",
  "",
];

const LINK = [
  "https://claude.com/cai/oauth/authorize?code=true&client_id=9d1c250a-e61b-44d9-88ed-5944d1962f5e&response_type=code&redirect_uri=https%",
  "3A%2F%2Fplatform.claude.com%2Foauth%2Fcode%2Fcallback&scope=org%3Acreate_api_key+user%3Aprofile+user%3Ainference+user%3Asessions%3Acla",
  `ude_code+user%3Amcp_servers+user%3Afile_upload+user%3Aplugins+user%3Aprivacy_consents%3Awrite&code_challenge=${X(25)}`,
  `${X(18)}&code_challenge_method=S256&state=${X(43)}`,
];

const code = [
  "Welcome to Claude Code v2.1.327",
  ".......█ █   █ █..........................................",
  "",
  " Browser didn't open? Use the URL below to sign in (c to copy)",
  "",
  ...LINK,
  "",
  " Hold Shift (Option in iTerm2, Fn in Terminal.app) while selecting to use your terminal's native copy",
  "",
  " Paste code here if prompted",
  " ╭───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮",
  " │                                                                                                                                   │",
  " ╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯",
  "",
  " Esc to go back",
  "",
];

// The same screen after a made-up code and Enter. No credential was written.
const wrong = [
  ".......█ █   █ █..........................................",
  "",
  " OAuth error: That code wasn't accepted. It may be incomplete, expired, already used, or from an earlier sign-in attempt. Sign in",
  " again to get a new code.",
  "",
  " Press Enter to retry. (Esc to continue without logging in)",
  "",
];

describe("detectLogin", () => {
  it("reads the logged-out start as the method menu and aims at the subscription row", () => {
    const login = detectLogin(method) as LoginMethod;
    expect(login.kind).toBe("method");
    expect(login.options).toHaveLength(3);
    expect(login.subscription).toBe(0);
    expect(login.cursor).toBe(0);
  });

  it("reads the sign-in screen, joining the wrapped link with nothing between its rows", () => {
    const login = detectLogin(code);
    expect(login).toEqual({ kind: "code", url: LINK.join("") });
  });

  it("joins the link the same way when a narrow terminal wraps it into more rows", () => {
    // Re-wrapped by the test at 44 columns. A narrow screen was not captured.
    const narrow = LINK.join("").match(/.{1,44}/g) ?? [];
    const at = code.indexOf(LINK[0]!);
    const rows = [...code.slice(0, at), ...narrow, ...code.slice(at + LINK.length)];
    expect(detectLogin(rows)).toEqual({ kind: "code", url: LINK.join("") });
  });

  it("reads a refused code with the seat's own words, joined across its wrapped row", () => {
    const login = detectLogin(wrong);
    expect(login?.kind).toBe("error");
    expect(login && "message" in login ? login.message : "").toBe("That code wasn't accepted. It may be incomplete, expired, already used, or from an earlier sign-in attempt. Sign in again to get a new code.");
  });

  it("tells the error from the link when a stale link row is still above it", () => {
    expect(detectLogin([...code.slice(0, 9), ...wrong])?.kind).toBe("error");
  });

  it("reads nothing off a seat that is signed in, a shell, or an empty screen", () => {
    expect(detectLogin([])).toBeNull();
    expect(detectLogin(["~ % ls", "src", "~ % "])).toBeNull();
    expect(detectLogin(["❯ ", "─".repeat(60), "  ⏵⏵ auto mode on (shift+tab to cycle)"])).toBeNull();
  });

  it("needs the subscription row and the highlight, so an unfamiliar menu is left to the terminal", () => {
    expect(detectLogin(method.filter((row) => !/subscription/.test(row) || /Claude Code can/.test(row)))).toBeNull();
    expect(detectLogin(method.map((row) => row.replace("❯", " ")))).toBeNull();
  });

  it("does not double up with the choice card on any of these screens", () => {
    for (const screen of [method, code, wrong]) expect(detectChoice(screen)).toBeNull();
  });

  it("keeps one card while the field fills, and a new one when the link or the kind changes", () => {
    const typed = code.map((row) => row.replace("│                                                      ", "│ abc#def                                              "));
    expect(loginKey(detectLogin(typed)!)).toBe(loginKey(detectLogin(code)!));
    expect(loginKey(detectLogin(wrong)!)).not.toBe(loginKey(detectLogin(code)!));
    expect(loginKey(detectLogin(method)!)).not.toBe(loginKey(detectLogin(code)!));
  });
});

describe("signInUrl", () => {
  it("opens only an https authorize link on a Claude host", () => {
    expect(signInUrl(LINK.join(""))).toBe(LINK.join(""));
    for (const bad of ["http://claude.com/cai/oauth/authorize", "https://claude.com.evil.example/cai/oauth/authorize", "https://evil.example/oauth/authorize?next=claude.com", "https://claude.com/cai/oauth/token", "https://user:pw@claude.com/oauth/authorize", "https://claude.com:8443/oauth/authorize", "javascript:alert(1)", "not a url", ""]) {
      expect(signInUrl(bad)).toBeNull();
    }
  });

  it("gives no link when the screen's link is not one the card may open", () => {
    const hostile = code.map((row) => (LINK.includes(row) ? row.replace("claude.com", "claude.example") : row));
    expect(detectLogin(hostile)).toEqual({ kind: "code", url: null });
  });
});

describe("keys", () => {
  it("presses Enter on the subscription row when the highlight is already there", () => {
    expect(methodKeys(detectLogin(method) as LoginMethod)).toEqual(["\r"]);
  });

  it("moves the highlight to the subscription row first, from either side", () => {
    const base = detectLogin(method) as LoginMethod;
    expect(methodKeys({ ...base, cursor: 2 })).toEqual(["\x1b[A\x1b[A", "\r"]);
    expect(methodKeys({ ...base, cursor: 0, subscription: 1 })).toEqual(["\x1b[B", "\r"]);
  });

  it("sends exactly the pasted code, then Enter", () => {
    expect(codeKeys("abc123#xyz_789-Q")).toEqual(["abc123#xyz_789-Q", "\r"]);
  });

  it("drops only the edges a keyboard adds, and sends nothing for an empty field", () => {
    expect(codeKeys("  abc#def \n")).toEqual(["abc#def", "\r"]);
    expect(codeKeys("a b#c")).toEqual(["a b#c", "\r"]);
    expect(codeKeys("   ")).toEqual([]);
  });

  it("retries with Enter and continues without a login with Esc", () => {
    expect(RETRY_KEYS).toEqual(["\r"]);
    expect(CONTINUE_KEYS).toEqual(["\x1b"]);
  });
});

describe("narrow screens", () => {
  // Wrapped by hand, as a narrow terminal would. A narrow login screen was not captured.
  const wrapped = [
    " Select login method:",
    "",
    " ❯ 1. Claude account with subscription ·",
    "      Individual, Team, or Enterprise",
    "   2. Anthropic Console account · API",
    "      usage billing",
    "   3. 3rd-party platform · Bedrock",
    "",
  ];

  it("keeps a wrapped option whole and still aims at the subscription row", () => {
    const login = detectLogin(wrapped) as LoginMethod;
    expect(login.options).toEqual(["Claude account with subscription · Individual, Team, or Enterprise", "Anthropic Console account · API usage billing", "3rd-party platform · Bedrock"]);
    expect([login.cursor, login.subscription]).toEqual([0, 0]);
  });

  it("reads the mock seat's three screens at a phone's width and a desktop's", () => {
    for (const cols of [34, 44, 70, 120]) {
      expect(detectLogin(loginFrame("method")(cols))?.kind).toBe("method");
      expect(detectLogin(loginFrame("code", "abc")(cols))).toEqual({ kind: "code", url: expect.stringMatching(/^https:\/\/claude\.com\/cai\/oauth\/authorize\?code=true.*state=demo-not-real$/) });
      expect(detectLogin(loginFrame("error")(cols))).toEqual({ kind: "error", message: expect.stringContaining("That code wasn't accepted.") });
    }
  });
});
