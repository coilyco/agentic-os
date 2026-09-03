#!/bin/sh
# Print the agentic_os module files aosguard guardfiles exec, one path per line.
# The guardfiles are the authority: a hand-kept copy drifts (agentic-os#6836).
set -eu

repo_root=${1:?repo root required}

{
    printf '%s/agentic_os/__init__.py\n' "$repo_root"
    grep -ho 'agentic_os\.[A-Za-z0-9_]*' \
        "$repo_root"/.umbra/guardfiles/aosguard/*.kdl |
        sort -u |
        sed "s|^agentic_os\.|$repo_root/agentic_os/|; s|\$|.py|"
} | sort -u
