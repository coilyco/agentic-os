#!/bin/sh
set -eu

usage() {
    echo "usage: $0 cloud|local ROLE [MODEL]" >&2
    exit 2
}

question_for() {
    case "$1" in
        engineer)
            echo "A CLI needs a dry-run flag without duplicating execution logic. Propose the smallest implementation shape and one decisive test."
            ;;
        director)
            echo "Two valuable initiatives compete for one engineering week. Give the decision rule, the evidence needed, and the explicit tradeoff."
            ;;
        qa)
            echo "A role parser currently accepts an empty role. Name the smallest adversarial test set and the pass condition."
            ;;
        advisor)
            echo "Two benchmark sources disagree about model latency. Give a short evidence-synthesis plan that preserves uncertainty."
            ;;
        ops)
            echo "Request latency doubled after a deployment with no error-rate change. Name the first three non-destructive checks and a rollback threshold."
            ;;
        pm)
            echo "A cross-repository feature needs design, implementation, and rollout. Break it into the smallest dependency-aware milestones."
            ;;
        designer)
            echo "A ten-role agent launcher must hide backend model identity. Shape the smallest clear role-selection interaction, including its default and failure state."
            ;;
        social)
            echo "Draft a restrained launch note for role-scoped agent skills and name one signal that would show the message helped."
            ;;
        sales)
            echo "A prospect says agent tooling feels too risky. Give one discovery question, one useful proof point, and one next step without pressure."
            ;;
        customer-success)
            echo "A new user cannot find the skill assigned to her role. Give the immediate support response and the product signal that should be routed upstream."
            ;;
        *)
            echo "aos role question: unknown role $1" >&2
            exit 2
            ;;
    esac
}

run_and_confirm() {
    role="$1"
    shift
    transcript="$(mktemp /tmp/aos-role-question.XXXXXX)"
    status_file="$(mktemp /tmp/aos-role-question-status.XXXXXX)"
    trap 'rm -f "$transcript" "$status_file"' 0 1 2 15
    (
        set +e
        "$@"
        status="$?"
        printf '%s\n' "$status" >"$status_file"
        exit 0
    ) 2>&1 | tee "$transcript"
    if [ ! -s "$status_file" ]; then
        echo "aos role question: probe ended without an exit status" >&2
        exit 1
    fi
    status="$(cat "$status_file")"
    if [ "$status" -ne 0 ]; then
        exit "$status"
    fi
    role_display="$(printf '%s' "$role" | tr '-' ' ')"
    if ! grep -Fqi "ROLE-CONFIRMED: ${role}" "$transcript" \
        && ! grep -Fqi "ROLE-CONFIRMED: ${role_display}" "$transcript"; then
        echo "aos role question: response did not confirm ${role}" >&2
        exit 1
    fi
    rm -f "$transcript" "$status_file"
    trap - 0 1 2 15
}

mode="${1:-}"
role="${2:-}"
model="${3:-qwen3.6:35b}"
aos_bin="${AOS_BIN:-./aos/aos}"
aos_image="${AOS_IMAGE:-agentic-os:aos-local}"

[ "$#" -ge 2 ] || usage
question="$(question_for "$role")"
prompt="Without using tools or editing files, identify the canonical role assigned by your loaded role instructions. Begin exactly with ROLE-CONFIRMED: followed by that role name. Then answer this real question in under 180 words: ${question}"

case "$mode" in
    cloud)
        run_and_confirm "$role" \
            "$aos_bin" \
                --role "$role" \
                --layout codex \
                --image "$aos_image" \
                --no-substrate \
                acompose -- \
                timeout 600 codex exec \
                    --ephemeral \
                    --sandbox read-only \
                    --skip-git-repo-check \
                    --color never \
                    "$prompt"
        ;;
    local)
        run_and_confirm "$role" \
            "$aos_bin" \
                --role "$role" \
                --layout goose \
                --image "$aos_image" \
                --no-substrate \
                acompose -- \
                timeout 600 env GOOSE_MODEL="$model" \
                    goose run --no-session -t "$prompt"
        ;;
    *)
        usage
        ;;
esac
