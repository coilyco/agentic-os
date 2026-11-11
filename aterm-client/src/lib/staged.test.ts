import { describe, expect, it } from "vitest";
import { clearsLine } from "./staged";

describe("clearsLine", () => {
  it("is Enter, Ctrl-C or Ctrl-U, the three the daemon counts", () => {
    expect(clearsLine("\r")).toBe(true);
    expect(clearsLine("\n")).toBe(true);
    expect(clearsLine("\x03")).toBe(true);
    expect(clearsLine("\x15")).toBe(true);
  });

  it("is not typing, backspace or an arrow, which leave a line on the shell", () => {
    expect(clearsLine("a")).toBe(false);
    expect(clearsLine("\x7f")).toBe(false);
    expect(clearsLine("\x1b[D")).toBe(false);
  });

  it("is not a newline inside a paste", () => {
    expect(clearsLine("\x1b[200~echo a\necho b\x1b[201~")).toBe(false);
    expect(clearsLine("\x1b[200~echo a\x1b[201~\r")).toBe(true);
  });
});
