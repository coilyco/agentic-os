#!/usr/bin/env bash
# Offline secret scan for the catalog pre-commit suite. See agentic-os#288.
#
# Wraps `trufflehog git file://.` with a path-exclude list so a trufflehog build
# that walks untracked working-tree files (its git source reads gitignored dirs
# like Rust `target/`, upstream trufflesecurity/trufflehog) cannot drown the
# scan in build-artifact false positives and block every commit fleet-wide.
#
# --exclude-paths (a regex file) is load-bearing over the inline --exclude-globs:
# only exclude-paths filters the git source's synthetic working/staged diff by
# path. --exclude-globs filters committed git-log objects alone, so it does NOT
# suppress the untracked-file read that causes the block (verified empirically,
# agentic-os#288). The regex set mirrors agentic-os-kai's CI trufflehog scan.
#
# Resolver lockfiles are scanned in two passes (agentic-os#8407). Their public
# sdist content hashes trip the SentryToken detector, so the first pass excludes
# them entirely; a second pass scans only the lockfiles with SentryToken off, so
# every other detector still guards them and a real credential in a lockfile (a
# registry token, a basic-auth URL) still fails. A secret anywhere else still
# fails.

exclude_paths_file="$(mktemp)"
include_paths_file="$(mktemp)"
cleanup() {
  rm -f "$exclude_paths_file" "$include_paths_file"
}
trap cleanup EXIT

# Conventionally-gitignored build/cache dirs, excluded from the first pass.
cat >"$exclude_paths_file" <<'EOF'
(^|/)target/
(^|/)\.venv/
(^|/)venv/
(^|/)node_modules/
(^|/)__pycache__/
(^|/)\.mypy_cache/
(^|/)\.pytest_cache/
(^|/)\.ruff_cache/
(^|/)(dist|build)/
EOF

# Resolver lockfiles: the single source of their names, shared by both passes.
cat >"$include_paths_file" <<'EOF'
(^|/)(uv\.lock|poetry\.lock|Pipfile\.lock|package-lock\.json|pnpm-lock\.yaml|yarn\.lock|Cargo\.lock)$
EOF

# The first pass also skips the lockfiles, so append the lockfile regex to the
# exclude set rather than repeat the list.
cat "$include_paths_file" >>"$exclude_paths_file"

# Git Bash creates a POSIX-style /tmp path, while the native Windows
# trufflehog binary needs the equivalent Windows path.
trufflehog_exclude_paths="$exclude_paths_file"
trufflehog_include_paths="$include_paths_file"
if command -v cygpath >/dev/null 2>&1; then
  trufflehog_exclude_paths="$(cygpath -w "$exclude_paths_file")"
  trufflehog_include_paths="$(cygpath -w "$include_paths_file")"
fi

# Pass 1: everything but build/cache dirs and lockfiles, every detector on.
trufflehog git file://. --since-commit HEAD \
  --exclude-paths "$trufflehog_exclude_paths" \
  --no-verification --no-update --fail
pass1=$?

# Pass 2: lockfiles only, with SentryToken off so the sdist-hash false positive
# is ignored while every other detector still guards the lockfiles.
trufflehog git file://. --since-commit HEAD \
  --include-paths "$trufflehog_include_paths" \
  --exclude-detectors SentryToken \
  --no-verification --no-update --fail
pass2=$?

if [[ "$pass1" -ne 0 || "$pass2" -ne 0 ]]; then
  exit 1
fi
