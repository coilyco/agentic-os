#!/bin/sh
set -eu

instructions="${HOME}/.codex/AGENTS.md"
skills_root="${HOME}/.agents/skills"
designer_skill="${skills_root}/tooling-designer-interaction-shaping/SKILL.md"

if [ ! -f "${instructions}" ]; then
    echo "designer smoke: missing Codex instructions at ${instructions}" >&2
    exit 1
fi
if ! rg -Fq "## designer - Product shaping." "${instructions}"; then
    echo "designer smoke: Codex instructions omit the designer role heading" >&2
    exit 1
fi
if ! rg -Fq "You are a designer." "${instructions}"; then
    echo "designer smoke: Codex instructions omit the designer briefing" >&2
    exit 1
fi
if [ ! -f "${designer_skill}" ]; then
    echo "designer smoke: selected composed skill was not promoted to SKILL.md" >&2
    exit 1
fi
if [ -e "${skills_root}/coding-shape-cli" ] ||
    [ -e "${skills_root}/coding-shape-web-server" ] ||
    [ -e "${skills_root}/tooling-qa-adversarial-verification" ]; then
    echo "designer smoke: another role's composed skill leaked into the catalog" >&2
    exit 1
fi
if rg --files "${skills_root}" | rg -q '(^|/)COMPOSED\.md$'; then
    echo "designer smoke: an unpromoted COMPOSED.md leaked into the catalog" >&2
    exit 1
fi

echo "ok: designer Codex home carries its role briefing and isolated composed skill"
