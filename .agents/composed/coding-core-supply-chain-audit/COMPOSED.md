---
name: coding-core-supply-chain-audit
description: Audit third-party packages, libraries, plugins, MCP servers, or upstream repos before pulling them in. Confirms maintainers real, code not malicious, project maintained.
allowed-tools: Bash Read Grep WebFetch
---

# Supply chain audit

## Triggers

audit this dep, vet this crate, is this safe, supply chain audit, trust check.

Vet a third-party package before it ships in your code. The output is a trust verdict (green / yellow / red) and a short writeup. Don't recommend "allow" without running the checks.

## When to run

Run this skill BEFORE:

- Adding a new direct dependency to `Cargo.toml`, `package.json`, `pyproject.toml`, `requirements.txt`, `go.mod`, `Pipfile`, `Brewfile`, or any other manifest.
- Installing a new MCP server (`claude mcp add` / `codex mcp add`, `~/.claude.json` / `~/.codex/config.toml`, or a plugin marketplace install).
- Adding a new GitHub Action (`uses: org/action@vX`) or workflow that pulls a third-party container.
- Pulling a new brew tap or formula.
- Cloning and running an upstream repo the human found in a search result, blog post, or social link.
- The harness denies a `cargo fetch` / `npm install` / `pip install` / similar action with a "guessed external dependency" or "untrusted external code" error.
- the human says "audit this," "vet this," "deep scan this," "is this real," or "should I trust this."

Skip this skill for:

- Bumping an existing pinned version of a dep that is already trusted (handled by dependabot review, not a fresh audit).
- Pulling from a known/trusted repo or registry.
- One-off `gh api` / `curl` reads of public docs.

## Verdict scale

- **Green** - Allow. Proceed normally. Ordinary maintenance risk (any dep can rot or get compromised later); use `cargo audit` / `npm audit` / dependabot for ongoing watch.
- **Yellow** - Allow with caveats. Document the yellow flags in the audit writeup so future-you knows the soft spots. Examples: bus factor of 1, very young project, single small maintainer.
- **Red** - Stop. Do not add. Surface findings to the human. Examples: typosquat, code suggests data exfiltration, account hijack signals, abandoned with no path forward, license mismatch with project.

## Audit checklist

The full multi-step checklist (org reality, maintainer signals, repo health, code review, downstream adoption, supply-chain risk patterns, build-script red flags, hijack patterns) lives in [`references/audit-checklist.md`](references/audit-checklist.md), with deeper-detail companions in [`references/maintainer-signals.md`](references/maintainer-signals.md), [`references/build-script-redflags.md`](references/build-script-redflags.md), and [`references/hijack-patterns.md`](references/hijack-patterns.md).

## More

- [Invoking cargo / npm above board](references/ward-pkg-wrappers.md) - route package-manager actions through `ward pkg`.
- [Output format](references/output-format.md) - the verdict form to write to chat, and what this skill is NOT.
