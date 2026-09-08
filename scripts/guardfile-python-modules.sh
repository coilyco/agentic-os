#!/bin/sh
# Print the agentic_os module files aosguard guardfiles exec, one path per line.
# From each argv `-m` operand only: prose names modules it never execs (#6836).
set -eu

repo_root=${1:?repo root required}

{
    printf '%s/agentic_os/__init__.py\n' "$repo_root"
    awk '
        /^[[:space:]]*argv[[:space:]]/ {
            count = split($0, token, "\"")
            for (i = 2; i <= count; i += 2)
                if (token[i] == "-m" && i + 2 <= count)
                    print token[i + 2]
        }
    ' "$repo_root"/.umbra/guardfiles/aosguard/*.kdl |
        grep '^agentic_os\.' |
        sort -u |
        sed "s|^agentic_os\.|$repo_root/agentic_os/|; s|\$|.py|"
} | sort -u
