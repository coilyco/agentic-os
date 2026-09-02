---
name: writing-voice-guide-linter
description: Lint prose against a voice profile you supply - deterministic regex rules with code-fence awareness and a strict exit. Triggers - voice lint, voice check, lint prose, house style check.
---

# voice-guide-linter

Pre-flight check for prose before it ships: commits, PRs, READMEs, issues,
posts, notes. Catches the high-frequency slips a house style names.

The engine holds no rules. Every value a house style could disagree about
lives in a profile, so this skill is reusable and the profile is the thing
that belongs to whoever wrote the style guide.

## Procedure

```sh
python3 <loaded-skill-dir>/writing-voice-guide-linter/lint.py \
  --profile <profile.json> <file-or-dir>... [--strict]
```

Reports each violation as `path:line: [rule] 'span' - hint`.

Resolve `<loaded-skill-dir>` from the current harness's native skill root.
The source checkout keeps the helper under `.agents/composed/`, while
agent-compose promotes it into the chosen role's native skills pack.

Default mode reports and exits zero. `--strict` exits non-zero on any
violation, suitable for pre-commit hook integration.

## Profile rule

A profile is JSON: an object with a non-empty `rules` list.

```json
{
  "name": "example-house-style",
  "rules": [
    {"id": "em-dash", "pattern": "—", "hint": "replace with ' - '"},
    {"id": "prose-table", "pattern": "^\\s*\\|.*\\|\\s*$", "scope": "line",
     "hint": "use flat bullets"}
  ]
}
```

- `id`, `pattern`, `hint` - required non-empty strings. `pattern` is a Python
  regex.
- `scope` - `span` (default) reports every match on the line. `line` matches
  the whole line, reports once, and skips the span rules for that line, which
  is what keeps a table row from also tripping every punctuation rule.
- `flags` - any of `i`, `m`, `s`.

A profile that will not load is an error instead of zero rules. Linting
nothing and reporting success is the failure mode this refuses.

## Notes

- Rules are regexes, so any rule needing a referent, a name, or judgement
  belongs in human review instead of here. A profile can still carry an
  over-flagging rule deliberately, and should say so in its own hint.
- Deciding which rules to carry is the profile's job. This skill has no
  opinion about em-dashes, pronouns, or anyone's address.
