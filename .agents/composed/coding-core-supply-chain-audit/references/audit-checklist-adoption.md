# Audit checklist: build scripts, deps, adoption, advisories

Steps 4-7 of the [audit checklist](audit-checklist.md). Capture findings into your final writeup. Steps 8-10 continue in [`audit-checklist-tail.md`](audit-checklist-tail.md).

### 4. Build scripts and proc macros (Rust-specific, but generalize)

This is the high-leverage check for malicious code. Build scripts run on every consumer's machine.

- **Rust:** Look for `build.rs` at the top level of the crate. Look for any path-dep proc-macro crates (these run at compile time inside rustc). Skim for: network calls (`reqwest`, `ureq`, `curl`, `wget`, sockets), filesystem writes outside `OUT_DIR`, environment-variable exfiltration, `std::process::Command` calls, `unsafe` in odd places.
- **npm:** Check `scripts.preinstall`, `scripts.install`, `scripts.postinstall` in `package.json`. These run on `npm install`. Same red flags as build.rs.
- **Python:** Check `setup.py` for non-trivial code (rare in modern projects, and `pyproject.toml` is safer). Check `pyproject.toml` for `[tool.poetry.scripts]` or `[project.scripts]` entries that point at suspicious binaries.
- **Go:** Look for `go:generate` directives that shell out to non-standard tools. Check for `init()` functions in published packages that do anything beyond simple registration.

If build scripts exist, **read them**. Don't just confirm they exist.

See [`build-script-redflags.md`](build-script-redflags.md) for concrete patterns.

### 5. Dependency tree

For Rust: read the package's full `Cargo.toml`. For npm: `package.json` plus a sanity check on `package-lock.json` if available.

- Are transitive deps from well-known maintainers (tokio-rs, hyperium, serde-rs, dtolnay, etc.)? Or do they pull from random forks?
- Any `path = "../foo"` or `git = "..."` deps that bypass the registry? Flag every one. A well-maintained project sometimes uses path deps for in-tree workspace crates (fine). A published crate that pulls from a contributor's fork-of-a-fork is a red flag.
- Total dep count. If a 200-line utility crate pulls in 80 transitive deps, ask why.

### 6. Downstream adoption

- **crates.io reverse deps:** `curl -s "https://crates.io/api/v1/crates/<name>/reverse_dependencies?per_page=20"` then read `meta.total`. Single-digit reverse deps on a young crate is normal. Zero is a yellow flag for anything claiming "production-ready."
- **GitHub code search:** `gh search code "<crate-name>::" filename:Cargo.toml --limit 30` - counts unique repos that import the package. Filter out the publisher's own repos.
- **Look for credible third-party adopters.** A recognizable user (a research lab, a university group, a known OSS project, a company you've heard of) is worth a dozen indie users.

### 7. Advisory and security databases

- RustSec Advisory DB: `curl -sL "https://api.github.com/repos/rustsec/advisory-db/contents/crates/<name>"` - 200 means an entry exists, 404 means clean.
- npm advisories: `npm audit` against a project that includes the dep, or `https://github.com/advisories?query=<package-name>+ecosystem%3Anpm`.
- GitHub Security Advisories: `gh api graphql` for `securityAdvisories` on the package's repo.
- For brew formulas, also check the maintainer-tap reputation. For MCP servers, also check whether the server is on Anthropic's curated list at modelcontextprotocol.io.

### Steps 8-10

External engagement, the recent-commit hijack check, and license sanity continue in [`audit-checklist-tail.md`](audit-checklist-tail.md).
