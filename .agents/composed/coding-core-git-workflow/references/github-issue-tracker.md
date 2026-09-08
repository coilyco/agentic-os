# GitHub issues as work tracker

Precedence: Kai's own work routes to the Teable tracker (see the default-todo reference). The rules here apply to repos with an active **GitHub** issue tracker, which are the external-facing repos where external contributors file. Forgejo issue trackers are off fleet-wide and are not a destination on either path.

When a coilysiren repo has an active issue tracker, issues are canonical - not scratch notes, TodoWrite, or harness memory.

- **Open issues for new requests** before starting. Enough detail to act cold, labels matching the repo's scheme, cross-links. **Don't ask first.** When a change in a repo with an active issue tracker needs a closing issue, file it and proceed. The answer is always yes. Only ask if the issue's framing is genuinely ambiguous (scope unclear, multiple plausible repos), not just to confirm filing it.
- **Close via commit subject** - `fixes #N` / `closes #N` auto-closes on push. For partial/tangential work, use `refs #N` and close manually: `gh issue close N --comment "<sha>: <one-liner>"`.
- **Tracker issues stay open.** When a commit's work is motivated by a long-lived tracker (a collector issue meant to accumulate cases, drifts, or TODOs over time), do not `closes` the tracker. File a *separate atomic issue* for the commit and `closes` that one. Confirmed 2026-05-14: a long-lived tracker issue got auto-closed by a commit that should have closed its own atomic implementation issue. Reopening works but pollutes the issue's state history.
- **Skip for trivia** - typos, formatting sweeps, one-liners. No issue tracker → don't manufacture one.

## Bot-attribution signature

Claude-filed issues (and Claude-written issue comments) are wrapped top and bottom with `> 🤖 Filed by Claude Code on Kai's behalf.` as a blockquote. Top line, blank line, body, blank line, bottom line. Same convention as the `Co-Authored-By` trailer on commits - makes attribution scannable without a separate bot account. Skip for Kai-authored content she's just asking Claude to post verbatim.

Check `gh issue list --repo coilysiren/<name>` when unsure.
