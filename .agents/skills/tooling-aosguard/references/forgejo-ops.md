# Forgejo ops and the GitHub mirror (retired)

Everything on this page retired with Forgejo on 2026-11-01 (COI-3075). GitHub is canonical for every repo (COI-3083). The guardfiles and code that still name these surfaces are cleanup under COI-2931.

## Retired surfaces

* **`aosguard ops forgejo actions generate-runner-token`** - minted Forgejo Actions runner registration tokens at `global`, `org` and `repo` scope. Forgejo runners are retired with it.
* **`aosguard ops forgejo-storage measure`** - the sealed, measurement-only storage report for the Forgejo application and its database. There is no Forgejo left to measure.
* **The GitHub mirror** - a timer in the infrastructure repo pushed Forgejo `main` to GitHub fast-forward only. There is no mirror now, because GitHub `main` is the source.

## History

The mirror job once ran `git push --force github main`, which GitHub rejected (`GH013`) under its cannot-force-push rule. Nobody watched mirror CI, so the mirror sat two weeks stale while Forgejo advanced 237 commits (agentic-os#309). That is a dated record of the old setup and is kept as written.
