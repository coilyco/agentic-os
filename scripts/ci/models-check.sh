#!/bin/sh
set -eu

# A scheduled run before the operational key exists skips with a warning, and
# a dispatched run still fails. See docs/build-file-headers.md.
if [ -z "${ANTHROPIC_MODELS_API_KEY:-}" ] && [ "${EVENT_NAME:-}" = "schedule" ]; then
    echo "::warning::ANTHROPIC_MODELS_API_KEY is not set, so the scheduled models check skipped (teable:coilyco-flight-deck/agentic-os#7838)"
    exit 0
fi
exec just aos-models-check
