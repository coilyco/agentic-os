# Pre-commit hygiene and the leak guard

This repo ships the baseline cleanliness hooks plus a few staged opt-ins for
text hygiene that are too disruptive to flip on everywhere at once.

## Active hooks

- the upstream hygiene set: `trailing-whitespace`, `end-of-file-fixer`,
  `check-added-large-files` (2048 KB), `check-merge-conflict`,
  `check-case-conflict`, `check-illegal-windows-names`, `mixed-line-ending`,
  `check-json`, `check-toml`
- `actionlint` on `.forgejo/workflows/*.yml` and `.yaml`. `.github/actionlint.yaml` teaches it the Forgejo runner label `docker`
- `actions-run-one-line` rejects block, folded, escaped-newline, and physically split `run:` commands in GitHub and Forgejo workflows plus composite actions, and rejects a program body inlined into one line. A tracked script, composite action, or `just` verb owns the implementation while YAML invokes it from one line. See [one line, and no inlined body](#one-line-and-no-inlined-body)
- `forgejo-runner-validate` for Forgejo-native workflow and local-action semantics
- `shellcheck` on shell scripts
- `typos` with repo-specific words in [`.typos.toml`](../.typos.toml)

## One line, and no inlined body

The one-line half alone is satisfiable by escaping a whole program into a
single string, cheaper than creating a file, so agents reach for it: 24
workflows in `coilyco-bridge/deploy` and 12 steps here landed one ~50-line
Python program as `python3 -c 'exec("import os\n...")'`, unreadable by `ruff`,
`shellcheck`, review, and `git diff`. Unshareable too, so one copy had drifted.

So the hook checks both halves. A `run:` fails when it opens a heredoc, or when
it passes an inline source string to `python`, `node`, `ruby`, `perl`, or a
shell through `-c`, `-e`, `-E`, `-p`, `--eval`, `--exec`, or `--print` and that
string carries an escaped newline or runs past `MAX_INLINE_BODY_CHARS` in
[`check_actions_run_one_line.py`](../agentic_os/pre_commit/check_actions_run_one_line.py).

A short direct one-liner still passes: the bar is naming a file to execute or
being a short direct command, not containing a program. Move the body to a
tracked script or a composite action under `actions/`, pass inputs through
`env:` or `with:`, and leave the step as the call.

## Outbound link hygiene

`dead-cross-links` returns early on anything carrying a scheme, so every link
leaving the repo was unchecked. `outbound-link-hygiene` takes that half, offline
because pre-commit must not depend on the network. Two renames earned it:
`ward-mcp` became `mcp-beaver` and `cli-guard` became `umbra` while the profile
README kept naming and linking both. Three `coilysiren.me/orgs/<org>/` links in
the same file had never resolved, found by checking 25 links by hand.

Four checks read [`outbound_link_rules.json`](../agentic_os/outbound_link_rules.json),
so a rename edits a table rather than a validator: retired names and paths,
canonical host for repository links, link text naming one project while the
target names another, and placeholder or local URLs. Fenced and inline code are
stripped first, so a doc narrating a rename backticks the retired name and one
still *using* it does not. That exemption is why paths keeping a pre-rename
spelling, such as SSM parameters, pass with no allowlist. Liveness is
`check-link-liveness`, a report-only CLI rather than a hook, and its scheduled
job is not built yet. Page-shape validation is deferred until the format has
more than one instance: its only draft is
[coilysiren/inbox#417](https://forgejo.coilysiren.me/coilyco-bridge/inbox/issues/417).

## Manual opt-ins

- `shfmt` - manual stage only. Shellcheck is the default gate because it is
  lower drama for the current shell style.
- `unresolved-placeholder-guard` - manual stage only. Use it once the repo has
  enough allowlists for examples and quoted snippets.
- `issue-reference-guard` - manual stage only. It skips fenced code, inline
  code, quoted command examples, and test fixtures, and it leaves external
  upstream issue links alone. Use it once the repo has a staged rollout plan
  and local allowlists for historical references.

Opting one in means adding it at `stages: [manual]` and supplying repo-local
config under `[tool.agentic-os.<hook>]` in `pyproject.toml`: `enabled = true`
plus optional `excludes` and `allow_globs` path lists. The issue guard is for
durable prose breadcrumbs like `See #337 for the draft`, not literal syntax
examples or upstream issue links. Manual-only hooks stay out of the fleet
coverage audit until they roll out as active checks.

## Encoded leak guard

`leak-guard` rejects plaintext occurrences of terms that should not be
grep-bait. Most leaks are not secrets, they are *awkward*: an employer name in a
config path, a partner's name in a comment, a private repo named in a public
README. None trip a secret scanner, but any turns `rg <term>` into a harvesting
tool. See the recovered inbox corpus.

Three leak and coupling classes reduce to one rule shape - *a string S must not
appear in scope T, and the rule is stored encoded so grepping the rule reveals
neither S nor the coupling*. **Sensitive data** is an employer or personal name
whose threat model is `rg <name> | mail-merge` rather than decryption, so a
reversible encoding is the right grade. **Private to public leak** is a bridge
(private) identifier referenced from a flight-deck (public) repo, the wrong
direction for data lockdown. **Dependency cycle** bans one direction of a
repo-to-repo reference so the edge stays one-way.

The ruleset (`agentic_os/pre_commit/leak_guard_rules.py`) stores every term as
lowercase **hex**, never plaintext. Hex beats base64 because it is exclusively
`[0-9a-f]` with no padding, copy-pastes cleanly, and decodes in one line in every
language. The hook decodes each term **only in memory** to build its matcher, and
a violation prints the rule id, path, line, and remediation - **never the term**,
so neither the ruleset nor the hook output is itself a leak. Terms match on word
boundaries, so a rule for `ward` does not fire on `forward` or `awkward`.

`leak-guard-encode` reads stdin and returns hex, so the plaintext never lands in
shell history, and `--decode <hex>` round-trips. Add the result to
`leak_guard_rules.py` as a rule carrying `id`, `term_hex`, optional `repos`,
optional `only_globs` and `allow_globs`, and a `message` that never names the
term. `only_globs` and `allow_globs` are duals: the first narrows a rule to a few
paths, the second exempts known-legitimate ones such as a public bio surface.
Both follow the catalog glob semantics against repo-relative POSIX paths, where
`**/x` does not match a top-level `x`. Per repo, opt paths out with
`[tool.agentic-os.leak-guard] excludes`.

Rule scope matches the current repo, resolved from `origin` so it is
worktree-safe. A rule with `repos` fires only there, and one without fires
everywhere it is installed. The hook is authored and dogfooded here, and fleet
rollout is a deliberate ansible step run after each target repo's occurrences are
cleaned or allowlisted. The guard is staged, never flipped on fleet-wide.

## Managed line endings

The rollout also writes a `.gitattributes` block pinning `* text=auto eol=lf`.
`text=auto` alone leaves it to `core.autocrlf`, splitting Windows from Linux.
