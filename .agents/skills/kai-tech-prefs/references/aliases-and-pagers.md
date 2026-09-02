# Aliases and pagers

## Don't shorten common command names

No `k=kubectl`, `gst=git status`, `kgp=kubectl get pods`, etc. Kai dislikes shortened-name aliases on principle: they make examples, screen recordings, and shared snippets inaccurate, and they break the "wrapper API mirrors the real CLI" instinct that drives ward's design.

- Multi-word convenience functions (e.g. `git-merge-default-branch`) are fine.
- Aliases that **flag** a command (`alias ls='ls -GFh'`) are fine.
- Aliases that **rename** it are not. Don't suggest them, don't add them to dotfiles, don't propose them in code review.

## No pagers

Pagers are a hard-no. `less`, `more`, `bat`'s default pager, `git`'s pager, anything that traps output behind a modal scroll surface gets configured off. Scrollback already gives clean output per command, so paging adds friction without adding value.

**Why:** every paged output requires `q` to exit, which interrupts the flow of fast terminal work. Surfaced as a hard preference during the 2025 terminal walkthrough and the bat-workflow recon.

**How to apply:**
- Any tool with a default pager gets configured off when wrapping or aliasing. `--no-pager` for git, `--paging=never` for bat, `PAGER=cat` for general escape, etc.
- When proposing CLI ergonomics, default to non-paged dumps. Don't pipe through `less` by default.
- If output is genuinely too long for the terminal, suggest piping through a pager explicitly rather than enabling one by default. The block model handles long blocks fine.
