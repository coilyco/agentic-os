# Build output is not repository content

A hook that walks the filesystem sees files git does not carry. That is how a
bake became a consuming repository's own skills.

## What went wrong

`ward exec compose-bundles` in sirens-echo writes `agent/bundles/`, which is
gitignored. The next `pre-commit` run reported 75 documentation-layout
violations and 8 dead links, all inside that bake, on an otherwise clean `main`.

Nothing was wrong with the repository. The layout violations were upstream
`agent-compose` skills landing at paths this repository's rules do not allow,
and the dead links were real relative links between catalogue skills that a
bundle does not carry, because a bundle holds only the skills its role admitted.
They are not fixable in the consuming repository and are not its content.

The gitignore did not help. These hooks run `always_run: true` with
`pass_filenames: false` and do their own walks, so pre-commit's file list and
its `exclude:` directive are both bypassed. See coilyco-gaming/sirens-echo#800.

## What decides it now

`agentic_os.config.is_build_output` asks git rather than guessing:

```
git ls-files -z --cached --others --exclude-standard
```

Tracked plus untracked-but-not-ignored is git's own definition of what the
repository holds, so the rule needs no pattern list and no per-repo opt-out. A
path outside that set is build output and no hook reads it.

Three properties are deliberate.

**It fails open.** No checkout, no git, or a failed call returns "this is
content" and every hook checks exactly what it checked before. A tarball, a
vendored copy, or a machine without git must never quietly stop being checked.
An empty answer counts as no answer for the same reason.

**A directory counts as content when anything under it does.** Directories
never appear in git's file list, and a rule shaped around one, such as `docs/`
flatness, would otherwise retire itself.

**Untracked is not ignored.** A new file the author has not staged is source,
not output. Skipping it would let a hook report a clean tree over work in
progress.

## Where it applies

`documentation-layout` (and its `catalog-doc-size` alias) and
`dead-cross-links`, the two hooks that fired. The other tree-walking hooks each
carry their own walk and are not converted here. See agentic-os#1062.

A per-repo `excludes` entry still works and still wins where a repo wants to
exempt content git does carry. The two are independent.

## See also

- [pre-commit hygiene](pre-commit-hygiene.md) - the per-repo config schema.
- [documentation layout exceptions](documentation-bands.md) - the
  hand-maintained escape hatch this rule makes unnecessary for build output.
