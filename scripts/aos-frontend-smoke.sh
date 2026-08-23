#!/bin/sh
set -eu

instructions="${HOME}/.codex/AGENTS.md"
skills_root="${HOME}/.agents/skills"
design_skill="${skills_root}/tooling-frontend-interaction-shaping/SKILL.md"
javascript_skill="${skills_root}/coding-javascript/SKILL.md"
react_skill="${skills_root}/coding-javascript-react/SKILL.md"
brainstorming_skill="${skills_root}/tooling-tpm-product-brainstorming/SKILL.md"

if [ ! -f "${instructions}" ]; then
    echo "design smoke: missing Codex instructions at ${instructions}" >&2
    exit 1
fi
if ! rg -Fq "role: design" "${instructions}"; then
    echo "design smoke: Codex instructions omit the v2 design role identity" >&2
    exit 1
fi
if ! rg -Fq "# Designer" "${instructions}"; then
    echo "design smoke: Codex instructions omit the Designer briefing" >&2
    exit 1
fi
if [ ! -f "${design_skill}" ]; then
    echo "design smoke: selected composed skill was not promoted to SKILL.md" >&2
    exit 1
fi
if [ ! -f "${javascript_skill}" ] || [ ! -f "${react_skill}" ]; then
    echo "design smoke: frontend coding skills were not promoted" >&2
    exit 1
fi
if [ ! -f "${brainstorming_skill}" ]; then
    echo "design smoke: product brainstorming was not promoted" >&2
    exit 1
fi
frontend_count=0
for coding_skill in "${skills_root}"/coding-*; do
    [ -e "${coding_skill}" ] || continue
    case "${coding_skill##*/}" in
        coding-javascript | coding-javascript-react)
            frontend_count=$((frontend_count + 1))
            ;;
        *)
            echo "design smoke: non-frontend coding skill leaked into the catalog" >&2
            exit 1
            ;;
    esac
done
if [ "${frontend_count}" -ne 2 ] ||
    [ -e "${skills_root}/tooling-eval-adversarial-verification" ]; then
    echo "design smoke: another role's composed skill leaked into the catalog" >&2
    exit 1
fi
if rg --files "${skills_root}" | rg -q '(^|/)COMPOSED\.md$'; then
    echo "design smoke: an unpromoted COMPOSED.md leaked into the catalog" >&2
    exit 1
fi

echo "ok: design Codex home carries its v2 briefing and frontend skill slice"
