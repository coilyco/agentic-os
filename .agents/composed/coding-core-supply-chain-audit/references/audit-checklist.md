## Audit checklist

Run every check unless the package is so trivial (e.g. a 50-line MIT-licensed utility crate from a 10-year-old account) that the verdict is obvious. Even then, document which checks you skipped and why.

For each check, capture findings into your final writeup. The writeup is the deliverable.

### 1. Identity and reputation of the org

Use `gh api orgs/<name>` for orgs, `gh api users/<name>` for individuals. Verify:

- Account is real, not a recently-created throwaway. **Account age over 1 year** is a soft floor. Under 6 months is a yellow flag, under 30 days is red unless it's an obvious mirror of a long-standing project.
- Public-facing identity matches stated identity. Cross-check `blog`, `homepage`, `email`, `twitter_username`, `bio`. Look for the org's website actually existing.
- Org type makes sense (Organization vs User account, and "Organization" alone doesn't mean verified).
- Public_repos count is non-zero and the repos look real, not all-empty.
- For **named individuals** mentioned in the bio: search for them. AWS Heroes, IETF authors, OSS maintainers of well-known projects, conference speakers all leave fingerprints. If the bio claims a named credential ("AWS ML Hero", "IETF chair", "Apache committer"), spot-check at least one.

### 2. Maintainer activity

Use `gh api repos/<owner>/<name>/contributors` and `gh api repos/<owner>/<name>/commits`.

- Top contributor's account age, follower count, and other-repo activity. Drive-by contributors count for nothing. Look at the **top 1-2** committers.
- Email addresses on commits should be consistent and match a real domain. Disposable-mail domains are a yellow flag.
- Most-recent commit date. **Under 12 months** is the bright line for "alive" (matches a "no dead repos" rule). Archived/maintenance-mode banners count as dead even if commits are recent.
- PR-vs-direct-push ratio. A solo maintainer pushing direct to main is fine. A maintainer who appears to merge their own PRs without review is also fine, but note it. A pattern of recent commits from many unverified email addresses to a previously-quiet repo is a hijack signal.

See [`references/maintainer-signals.md`](maintainer-signals.md) for full rubric.

### 3. Repo health artifacts

Use `gh api repos/<owner>/<name>/contents` to list top-level files. Healthy projects have most of these:

- `LICENSE` (and not a confusing dual / weird custom one)
- `CHANGELOG.md` or `RELEASES.md` with real version history
- `.github/workflows/` with CI that actually runs
- `dependabot.yml` or `renovate.json`
- `deny.toml` (cargo) / `audit-ci.json` (npm) / `pip-audit` config - supply-chain hygiene tools
- `SECURITY.md` for non-trivial projects
- `.pre-commit-config.yaml` or equivalent linting config

Missing one or two is fine. Missing all of them on a 5,000-LOC dep is a yellow flag.

### Steps 4-10

Build scripts and proc macros, the dependency-tree check, adoption, advisory databases, external engagement, the hijack commit-pattern check, and license sanity continue in [`audit-checklist-adoption.md`](audit-checklist-adoption.md).
