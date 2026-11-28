import { describe, expect, it } from "vitest";
import { isClaudeSignIn, isSignInPage } from "./sign-in-pages";

const ACCOUNTS = "https://accounts.treehouse.example";

describe("isSignInPage", () => {
  it("is a Claude, Google, GitHub or Treehouse accounts sign-in page", () => {
    for (const url of [
      "https://claude.ai/oauth/authorize?code=true&state=S",
      "https://claude.com/cai/oauth/authorize?state=S",
      "https://www.claude.com/oauth/authorize",
      "https://platform.claude.com/oauth/authorize?state=S",
      "https://accounts.google.com/o/oauth2/v2/auth?client_id=x",
      "https://accounts.google.com/",
      "https://github.com/login",
      "https://github.com/login/device",
      "https://github.com/login/device/success",
      "https://github.com/login/oauth/authorize?client_id=x",
      "https://github.com/session",
      "https://github.com/sessions/two-factor/app",
      `${ACCOUNTS}/signin?state=S`,
    ]) {
      expect(isSignInPage(url, ACCOUNTS), url).toBe(true);
    }
  });

  it("is nothing else, lookalike hosts and plain pages on the same sites included", () => {
    for (const url of [
      "https://claude.ai/chat/abc",
      "https://claude.ai/artifact/abc",
      "https://claude.ai.evil.example/oauth/authorize",
      "http://claude.ai/oauth/authorize",
      "https://github.com/coilyco/agentic-os",
      "https://github.com/login-help",
      "https://accounts.google.com.evil.example/",
      "https://accounts.example.com/login",
      "https://accounts.treehouse.example.evil/signin",
      "not a url",
    ]) {
      expect(isSignInPage(url, ACCOUNTS), url).toBe(false);
    }
  });

  it("takes no accounts origin when the build names none", () => {
    expect(isSignInPage(`${ACCOUNTS}/signin`, null)).toBe(false);
  });
});

describe("isClaudeSignIn", () => {
  it("is only Claude's authorize page", () => {
    expect(isClaudeSignIn("https://claude.ai/oauth/authorize?state=S")).toBe(true);
    expect(isClaudeSignIn("https://accounts.google.com/")).toBe(false);
  });
});
