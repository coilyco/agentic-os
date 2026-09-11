# Maintainer signals rubric

Quick rubric for grading the maintainer side of an audit.

## Account age

| Age | Signal |
|---|---|
| 5+ years | Strong positive. Pattern of activity is established. |
| 2-5 years | Positive. Treat as normal. |
| 1-2 years | Neutral. Check for credible cross-references. |
| 6-12 months | Yellow flag. Maintainer is new but credible if other-repo activity is real. |
| < 6 months | Yellow flag. Demand stronger external corroboration. |
| < 30 days | Red flag for any non-trivial dep. Possible burner / typosquat. |

Account age alone is not disqualifying. A real human can have a new GitHub account (for example, after a name change). But couple it with other yellow signals and the verdict shifts.

## Follower count

Followers are a weak proxy for "this person is known in their field." A 10-year-old account with 5 followers is probably an indie dev, which is fine. A claim like "AWS Hero" or "Apache committer" with 5 followers warrants spot-checking.

| Followers | Reading |
|---|---|
| 1000+ | Public-figure tier. Reputational stakes are real. |
| 100-1000 | Active in their field. |
| 10-100 | Indie / niche. Common, fine. |
| < 10 | Verify other signals before trusting. |

## Other-repo activity

The single best signal: **does the maintainer have a public history of doing the kind of work this dep does**?

A maintainer publishing a Rust async runtime crate should have other Rust async work in their history. A maintainer publishing a cryptography library should have other crypto work. A first-time author publishing a 5,000-LOC niche library is a yellow flag, even with a credible account.

Check via:

```sh
gh api users/<handle>/repos --jq '.[] | "\(.name)\t\(.language)\t\(.stargazers_count)\t\(.pushed_at[:10])"' | head -20
```

## Email-on-commit consistency

Run:

```sh
gh api repos/<owner>/<name>/commits --jq '.[] | .commit.author.email' | sort -u
```

Healthy patterns:

- 1-3 emails total (maintainer's personal + work + GitHub noreply)
- All from real domains (gmail.com, gh-noreply, the org's domain)
- New committers introduced gradually over time

Yellow flags:

- Sudden burst of new committer emails in the last 30 days, especially on a previously-quiet repo
- Disposable-mail domains (mail.tm, guerrillamail, 10minutemail)
- Email mismatches with PR-author handles (account-takeover signal)

## Direct push vs PR-merge ratio

Both are fine in different contexts:

- **Solo project**: direct push to main is normal and arguably more honest than fake-PR theatre. some maintainers push direct to main on their own repos.
- **Multi-maintainer project**: PR-with-review is the expectation. A solo maintainer of a multi-maintainer-shaped repo who suddenly drops review and pushes direct is a process-degradation signal.
- **Dependabot-only PRs**: fine. Means the project automates dep bumps.

The judgment is "does the process match the project's stated shape," not "more PRs is more cred."

## Bus factor

Single maintainer = bus factor 1. Always document this. It's not disqualifying but it informs mitigation:

- For a bus-factor-1 dep that you depend on critically, plan for the fork. Note it in the audit. Consider mirroring to coilysiren.
- For a bus-factor-1 dep that's load-bearing for a recruiter-facing project, the maintenance risk is also a **career** risk. A stalled dep can date the project.

## Named-credential verification

If the bio claims a public credential, spot-check at least one against the directory that would list it. The per-credential lookup table continues in [`maintainer-credentials.md`](maintainer-credentials.md).
