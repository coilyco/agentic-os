import { describe, expect, it } from "vitest";
import { runShellLine } from "./demo-shell";

describe("runShellLine", () => {
  it("echoes what it was given", () => {
    expect(runShellLine("echo ok  there")).toEqual({ output: "ok there\r\n" });
  });

  it("prints nothing for an empty line and says so for an unknown command", () => {
    expect(runShellLine("   ")).toEqual({ output: "" });
    expect(runShellLine("kubectl get pods").output).toBe("zsh: command not found: kubectl\r\n");
  });

  it("ends the shell on exit, with the code it was given or zero", () => {
    expect(runShellLine("exit").exit).toBe(0);
    expect(runShellLine("exit 2").exit).toBe(2);
    expect(runShellLine("exit nope").exit).toBe(0);
  });
});
