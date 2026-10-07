#!/usr/bin/env bash
# Offline secret scan for the catalog pre-commit suite, in two modes.
# See agentic-os#288, COI-2134 and COI-2430.
#
# At pre-commit it scans the staged index (below). At pre-push there is no index, so
# it scans the commits being pushed instead: pre-commit exports PRE_COMMIT_FROM_REF and
# PRE_COMMIT_TO_REF for a ref range, and only PRE_COMMIT_REMOTE_NAME for a first push
# of a branch that reaches the root commit, which scans the whole history. COI-2430
# measured the last 50 commits of agentic-os, agentic-os-kai and agent-proxy and found
# no findings, so a push-time finding blocks. The push scan keeps the commit-stage path
# lists and lockfile split below. A secret added and removed inside the range still fails.
#
# A push scan that reads nothing is a failure. `trufflehog git` execs `git log` itself and
# exits 0 with `chunks: 0` when that fails. Under the agent commit-attribution shim it
# does: trufflehog gives git no PATH and the shim needs one to find the real git. The
# script drops that shim dir for the scan and fails any non-empty range that reads no
# chunks, so another cause cannot pass silently. The same silent pass hit the first real
# push: pre-commit exports PRE_COMMIT=1, and trufflehog git then ignores its range flags
# and scans only staged changes. The scan runs with PRE_COMMIT unset.
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
# fails under every detector. The push scan keeps the same split and the same path
# lists, so the two stages agree on what is a false positive.

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

# Push-time range scan. A non-empty range that reads no chunks is a failed scan, not a
# clean one: trufflehog exits 0 when its own `git log` fails (see the header).
scan_range() {
  local to_ref="${PRE_COMMIT_TO_REF:-HEAD}" from_ref="${PRE_COMMIT_FROM_REF:-}"
  local range="$to_ref" since=()
  if [[ -n "$from_ref" ]]; then
    range="$from_ref..$to_ref"
    since=(--since-commit "$from_ref")
  fi
  local commits
  commits="$(git rev-list --count "$range")" || return 1
  [[ "$commits" -gt 0 ]] || return 0

  printf '%s\n' "${build_cache_patterns[@]}" "$lockfile_pattern" >"$scan_root/skip.paths"
  printf '%s\n' "$lockfile_pattern" >"$scan_root/lock.paths"
  local common=(git "file://$PWD" "${since[@]}" --branch "$to_ref" --no-verification --no-update --fail)

  # The commit-attribution shim cannot find git under trufflehog and the log read then
  # fails silently. Nothing here is a commit to attribute, so drop it (COI-2430).
  local path_clean
  path_clean="$(printf '%s' "$PATH" | tr ':' '\n' | grep -v '/agent-git-attribution$' | paste -sd: -)"

  local rc=0 chunks
  env -u PRE_COMMIT PATH="$path_clean" trufflehog "${common[@]}" --exclude-paths "$scan_root/skip.paths" >"$scan_root/pass1.out" 2>&1
  [[ $? -eq 0 ]] || rc=1
  chunks="$(grep -o '"chunks": [0-9]*' "$scan_root/pass1.out" | tail -1 | grep -o '[0-9]*$')"
  if [[ "${chunks:-0}" -eq 0 ]]; then
    echo "trufflehog read no chunks from $commits commit(s) in $range, so the push scan did not run" >&2
    rc=1
  fi
  env -u PRE_COMMIT PATH="$path_clean" trufflehog "${common[@]}" --include-paths "$scan_root/lock.paths" \
    --exclude-detectors SentryToken >"$scan_root/pass2.out" 2>&1 || rc=1

  [[ "$rc" -eq 0 ]] || cat "$scan_root/pass1.out" "$scan_root/pass2.out"
  return "$rc"
}

if [[ -n "${PRE_COMMIT_TO_REF:-}" || -n "${PRE_COMMIT_REMOTE_NAME:-}" ]]; then
  scan_range
  exit $?
fi

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
