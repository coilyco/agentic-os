# Where a "!" line runs

The same `!` does two things in Treehouse (COI-3802). In a seat's composer, most harnesses take a leading `!` as a shell command in the seat's own session, so the seat sees the command and its output. In the shell beside the seat, `!` is only a character of a plain shell, and no agent reads that shell. Kai, 2026-11-27: nothing on screen said which box she was typing into.

## What shows

* **The seat composer** - while the draft starts with `!` (after leading spaces, which a send trims), a quiet line floats above the field, for example `Runs in Breezy Griffin's shell. The seat sees the output.` It floats so the seat's terminal, and so its PTY, never resize when it comes and goes. The @-mention and Recent sends lists take its place while open.
* **The shell beside the seat** - the pane's own label, `Shell beside <seat>`, reads `Runs in a plain terminal. No agent sees it.` instead. A floating line covered a fresh shell's prompt, the very line being typed. Two lines at 320px fit inside the bar's existing height.
* **Screen readers** - each box has a status region mounted from the first frame, so the line is announced once when it appears, and the composer's field is described by it. The visible copy is `aria-hidden`.
* **Never blocking** - neither line takes focus or disables Send.

## Which harness does what

Observed 2026-11-27 by pasting `! echo` into each harness in a pty with `testbed/record-harness.py`, as the composer sends it (one bracketed paste, then Enter).

* **claude** (Claude Code 2.1.332) - enters its shell mode, runs the command, and the output sits in the seat's conversation.
* **codex** - enters `Shell mode`, runs it, and records `You ran` with the output in the conversation.
* **opencode** (1.19.0) and **goose** (1.52.0) - send the text to the model as a message, so the line says that.
* **any other harness** - the line says the text goes to the seat and that its `!` handling is not checked. gemini was not installed to try.

The table is `SHELL_MODE` and `AS_MESSAGE` in `src/lib/bang.ts`. A harness release can change this, so check again with the recorder before trusting the table for a new version.

## The shell's line

A shell has no text field, so `trackLine` follows what is typed: printable keys append, Backspace removes, Enter, Ctrl-C and Ctrl-U start a fresh line, and a paste's last line is kept. An arrow, Tab or any other key the shell edits the line with makes the line unknown, and an unknown line shows no hint until the next fresh line. A staged command (COI-3306) is also unknown, since the daemon typed it.

## Where it lives

`src/lib/bang.ts` and its unit test, `src/components/BangHint.svelte` (the composer's line), `Composer.svelte`, `TerminalPane.svelte`. The journey is `bang-hint`, at every phone and desktop project.
