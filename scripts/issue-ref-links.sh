#!/usr/bin/env bash
# Stop hook: find every issue/PR hash-ref in the reply just written and record
# the URL that would resolve it. Warn mode logs, block mode asks for a rewrite.

# Warn is the default because 46.7% of turn-end messages carry a ref, so blocking
# from day one would refuse every other reply. Rates, router and log shape:
# teable:coilyco-flight-deck/agentic-os#7246.
set -uo pipefail

mode="${AOS_ISSUE_REF_MODE:-warn}"
[ "$mode" = "off" ] && exit 0

# $HOME moves per native shadow session, so a log keyed to it fragments into one
# file per session. The rollout passes an absolute path.
log_file="${AOS_ISSUE_REF_LOG:-${HOME:-/tmp}/.claude/issue-ref-links.log}"

record_reader="https://claude.ai/code/artifact/f6de33ad-e4d9-4a46-9de6-bef8d986e8e6"
forgejo="https://forgejo.coilysiren.me"

# Microseconds, or empty where the shell has no high-resolution clock, which is
# bash 3.2 on macOS. Absent, the log carries elapsed_ms=na.
now_us() {
  if [ -n "${EPOCHREALTIME:-}" ]; then
    local t=${EPOCHREALTIME/,/.}
    printf '%s%s' "${t%.*}" "${t#*.}"
  fi
}

started=$(now_us)

log() {
  { mkdir -p "$(dirname "$log_file")" 2>/dev/null \
      && printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >>"$log_file"; } 2>/dev/null || true
}

elapsed() {
  local end
  end=$(now_us)
  if [ -n "$started" ] && [ -n "$end" ]; then
    printf '%s' $(( (end - started) / 1000 ))
  else
    printf 'na'
  fi
}

payload=$(cat)

# One jq call, three values. The message is last because it is the only one that
# can contain newlines, so everything after the second line belongs to it.
parsed=$(printf '%s' "$payload" | jq -r '
  (.stop_hook_active // false), (.transcript_path // ""), (.last_assistant_message // "")
' 2>/dev/null)

active=${parsed%%$'\n'*}
rest=${parsed#*$'\n'}
transcript=${rest%%$'\n'*}
case "$rest" in
  *$'\n'*) msg=${rest#*$'\n'} ;;
  *) msg="" ;;
esac

# Already blocked once in this stop sequence. Let it through so a message that
# cannot be fixed, such as one quoting the convention, cannot wedge the session.
if [ "$active" = "true" ]; then
  log "mode=$mode decision=pass reason=loop-guard elapsed_ms=$(elapsed)"
  exit 0
fi

# At Stop entry the transcript has not been flushed yet, which is what made the
# retired hook read empty on 94.7% of events. Fallback only, never the primary.
if [ -z "$msg" ] && [ -n "$transcript" ] && [ -f "$transcript" ]; then
  msg=$(tail -n 200 "$transcript" 2>/dev/null | jq -rs '
    [ .[]? | select(.type=="assistant") | (.message.content // [])
      | if type=="array" then (.[] | select(.type=="text") | .text) else tostring end ]
    | last // ""
  ' 2>/dev/null || true)
fi

if [ -z "$msg" ]; then
  log "mode=$mode decision=pass reason=empty-message elapsed_ms=$(elapsed)"
  exit 0
fi

fenced_stripped=$(printf '%s\n' "$msg" | sed -e '/^[[:space:]]*```/,/^[[:space:]]*```/d')
# Only to decide prose or code span. Inline code is NOT exempt: exempting it
# hides a ref in 18.8% of every ref-bearing message.
inline_stripped=$(printf '%s\n' "$fenced_stripped" | sed -e 's/`[^`]*`//g')

ref_re='(teable:)?[A-Za-z0-9._-]+/[A-Za-z0-9._-]+#[0-9]+|teable:#[0-9]{2,}|(^|[^[:alnum:]&#])#[0-9]{2,}'

refs=$(printf '%s\n' "$fenced_stripped" \
  | grep -oE "$ref_re" 2>/dev/null \
  | sed -E 's/^[^[:alnum:]#]//' \
  | sort -u)

if [ -z "$refs" ]; then
  log "mode=$mode decision=pass reason=no-refs elapsed_ms=$(elapsed)"
  exit 0
fi

fields=""
targets=""
ambiguous=0
count=0

while IFS= read -r ref; do
  [ -n "$ref" ] || continue
  n=${ref##*#}
  case "$ref" in
    teable:*/*#*) kind=record;      target="$record_reader?n=$n#$n" ;;
    teable:#*)    kind=record-bare; target="$record_reader?n=$n#$n" ;;
    */*#*)        kind=forgejo-pull; target="$forgejo/${ref%#*}/pulls/$n" ;;
    \#*)          kind=ambiguous;   target="" ; ambiguous=1 ;;
    *)            continue ;;
  esac

  case "$inline_stripped" in
    *"$ref"*) where=prose ;;
    *) where=code ;;
  esac

  count=$((count + 1))
  fields="$fields ref=$ref|$kind|$where"
  if [ -n "$target" ]; then
    targets="$targets
  $ref -> $target"
  else
    targets="$targets
  $ref -> ambiguous, qualify it as teable:<owner>/<repo>#$n for a tracker record, <owner>/<repo>#$n for a Forgejo pull request, or teable:#$n when you hold the number but not the org"
  fi
done <<EOF
$refs
EOF

if [ "$count" -eq 0 ]; then
  log "mode=$mode decision=pass reason=no-refs elapsed_ms=$(elapsed)"
  exit 0
fi

if [ "$mode" != "block" ]; then
  log "mode=$mode decision=warn refs=$count ambiguous=$ambiguous elapsed_ms=$(elapsed)$fields"
  exit 0
fi

# Forgejo issue trackers are off fleet-wide, so an /issues/ URL here would make
# every agent on the fleet emit a dead link. Pull requests are live.
reason="Your reply referenced an issue or pull request by hash-ref, which costs the reader a browser and three clicks to resolve. Rewrite each one as the link beside it and resend:
$targets

A tracker record reads at the record reader. A Forgejo ref is a pull request. A bare number says neither, so name which one it is rather than leaving the reader to guess."

log "mode=$mode decision=block refs=$count ambiguous=$ambiguous elapsed_ms=$(elapsed)$fields"
jq -nc --arg r "$reason" '{decision:"block", reason:$r}'
exit 0
