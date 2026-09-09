#!/usr/bin/env bash
# Stop-hook probe for teable:coilyco-flight-deck/agentic-os#7246. It measures and
# never blocks: the 94.7% empty read is either a flush race or a jq defect, and
# one instrumented pass separates them. See docs/build-file-headers.md.

# Logs lengths, counts and timings only, never transcript text, because the
# thing being measured is full of other people's replies and secrets.
set -uo pipefail

log_file="${PROBE_STOP_LOG:-${HOME:-/tmp}/.claude/probe-stop-hook.log}"
# Sleeping taxes every turn end on this host, so the probe retires itself rather
# than relying on someone remembering to unwire it.
sleep_budget="${PROBE_STOP_BUDGET:-150}"

log() {
  { mkdir -p "$(dirname "$log_file")" 2>/dev/null \
      && printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >>"$log_file"; } 2>/dev/null || true
}

now_ms() { perl -MTime::HiRes=time -e 'printf "%.0f", time()*1000' 2>/dev/null || echo 0; }

# The retired extraction, unchanged, so its cost and output are the baseline.
extract() {
  jq -rs '
    [ .[] ] as $a
    | ([ $a | to_entries[] | select(.value.type=="user") | .key ] | last // -1) as $lu
    | [ $a[($lu + 1):][]
        | select(.type=="assistant")
        | (.message.content // [])
        | if type=="array" then (.[] | select(.type=="text") | .text)
          else tostring end ]
    | join("\n")
  ' 2>/dev/null || true
}

input=$(cat)
transcript=$(printf '%s' "$input" | jq -r '.transcript_path // empty' 2>/dev/null || true)
active=$(printf '%s' "$input" | jq -r '.stop_hook_active // false' 2>/dev/null || true)

# Evie's question: does the harness hand the reply over directly? Key names with
# value lengths answer it without putting any value in the log.
shape=$(printf '%s' "$input" \
  | jq -r 'to_entries | map("\(.key):\(.value|tostring|length)") | join(",")' 2>/dev/null || true)

if [ -z "$transcript" ] || [ ! -f "$transcript" ]; then
  log "event active=${active} shape=${shape:-<none>} transcript=absent"
  exit 0
fi

t0_bytes=$(wc -c <"$transcript" 2>/dev/null | tr -d ' ' || echo 0)

a=$(now_ms); slurp=$(extract <"$transcript"); b=$(now_ms)
slurp_ms=$((b - a))
slurp_len=${#slurp}

# Evie's tail -n 200 variant, swept identical on 48 transcripts. Measured here
# against live traffic rather than trusted from the sweep.
c=$(now_ms); tailed=$(tail -n 200 "$transcript" 2>/dev/null | extract); d=$(now_ms)
tail_ms=$((d - c))
tail_len=${#tailed}
agree=$([ "$slurp" = "$tailed" ] && echo same || echo diff)

# Only the empty case is in question, and only while budget remains, so a turn
# that already read its text pays nothing.
settled=skipped
t1_bytes="$t0_bytes"
slurp_len_t1="$slurp_len"
if [ "$slurp_len" -eq 0 ]; then
  seen=0
  [ -f "$log_file" ] && seen=$(wc -l <"$log_file" 2>/dev/null | tr -d ' ')
  if [ "${seen:-0}" -lt "$sleep_budget" ]; then
    sleep 1
    t1_bytes=$(wc -c <"$transcript" 2>/dev/null | tr -d ' ' || echo 0)
    after=$(extract <"$transcript")
    slurp_len_t1=${#after}
    settled=$([ "$t1_bytes" -gt "$t0_bytes" ] && echo grew || echo unchanged)
  else
    settled=budget-spent
  fi
fi

log "event active=${active} shape=${shape:-<none>}" \
  "t0_bytes=${t0_bytes} t1_bytes=${t1_bytes} settled=${settled}" \
  "slurp_len=${slurp_len} slurp_ms=${slurp_ms}" \
  "tail_len=${tail_len} tail_ms=${tail_ms} agree=${agree}" \
  "slurp_len_t1=${slurp_len_t1}"
exit 0
