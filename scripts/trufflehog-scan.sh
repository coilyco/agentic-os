#!/usr/bin/env bash
# Offline secret scan of the staged index for the catalog pre-commit suite.
# See agentic-os#288 and COI-2134.
#
# The index is written out with `git checkout-index` and that directory is scanned
# with `trufflehog filesystem`. The earlier `trufflehog git file://. --since-commit
# HEAD` scanned a temp clone, and a clone carries no staged changes, so a staged
# credential reported `chunks: 0` and passed (COI-2134). Reading the index also
# makes the scan independent of untracked and gitignored working-tree files, which
# is what the old path-exclude list defended against (agentic-os#288).
#
# Conventionally-gitignored build/cache dirs are skipped here when force-added,
# matched against repo-relative paths. Handing the regexes to trufflehog would match
# them against the absolute temp path, so a temp dir named build/ would silently
# exclude everything.
#
# Resolver lockfiles are scanned in a separate pass (agentic-os#8407). Their public
# sdist content hashes trip the SentryToken detector, so that pass runs with
# SentryToken off and every other detector still guards them: a real credential in a
# lockfile (a registry token, a basic-auth URL) still fails. A secret anywhere else
# fails under every detector.

# Any scan failure must fail the hook, so no `set -e`: both passes always run.
cd "$(git rev-parse --show-toplevel)" || exit 1

build_cache_patterns=(
  '(^|/)target/'
  '(^|/)\.venv/'
  '(^|/)venv/'
  '(^|/)node_modules/'
  '(^|/)__pycache__/'
  '(^|/)\.mypy_cache/'
  '(^|/)\.pytest_cache/'
  '(^|/)\.ruff_cache/'
  '(^|/)(dist|build)/'
)
lockfile_pattern='(^|/)(uv\.lock|poetry\.lock|Pipfile\.lock|package-lock\.json|pnpm-lock\.yaml|yarn\.lock|Cargo\.lock)$'

scan_root="$(mktemp -d)"
cleanup() {
  rm -rf "$scan_root"
}
trap cleanup EXIT

mkdir "$scan_root/files" "$scan_root/lockfiles"
: >"$scan_root/files.list"
: >"$scan_root/lockfiles.list"

# Added, copied, modified, and renamed paths only: a deletion has no content to leak.
while IFS= read -r -d '' path; do
  if [[ "$path" =~ $lockfile_pattern ]]; then
    printf '%s\0' "$path" >>"$scan_root/lockfiles.list"
    continue
  fi
  skip=0
  for pattern in "${build_cache_patterns[@]}"; do
    if [[ "$path" =~ $pattern ]]; then
      skip=1
      break
    fi
  done
  [[ "$skip" -eq 1 ]] || printf '%s\0' "$path" >>"$scan_root/files.list"
done < <(git diff --cached --name-only -z --diff-filter=ACMR)

# Scan from inside the directory so findings print repo-relative paths.
scan_dir() {
  local name="$1"
  shift
  [[ -s "$scan_root/$name.list" ]] || return 0
  git checkout-index --prefix="$scan_root/$name/" -z --stdin <"$scan_root/$name.list" || return 1
  (cd "$scan_root/$name" && trufflehog filesystem . "$@" --no-verification --no-update --fail)
}

# Pass 1: everything but build/cache dirs and lockfiles, every detector on.
scan_dir files
pass1=$?

# Pass 2: lockfiles only, with SentryToken off so the sdist-hash false positive is
# ignored while every other detector still guards the lockfiles.
scan_dir lockfiles --exclude-detectors SentryToken
pass2=$?

if [[ "$pass1" -ne 0 || "$pass2" -ne 0 ]]; then
  exit 1
fi
