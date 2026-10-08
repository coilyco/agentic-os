// The demo host's shell: a few commands, enough to see a terminal take input and exit.
// It reads nothing and runs nothing. Real shells come from the daemon.

export const SHELL_PROMPT = "demo@host ~ % ";

/** What one entered line prints, and an exit code when it ends the shell. */
export interface ShellStep {
  output: string;
  exit?: number;
}

export function runShellLine(line: string): ShellStep {
  const [command = "", ...rest] = line.trim().split(/\s+/);
  switch (command) {
    case "":
      return { output: "" };
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
      return { output: `zsh: command not found: ${command}\r\n` };
  }
}
