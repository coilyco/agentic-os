#!/usr/bin/env python3
"""leak-guard ruleset: terms held as hex, decoded only in memory by the check.

Each rule bans a term in a scope. The term is lowercase hex (see
`leak-guard-encode`) so a plain `rg <term>` over this file finds nothing - the
ruleset honors the guard it drives. Add a sensitive term without it touching
shell history:

    leak-guard-encode            # type or pipe the term, copy the hex

Rule fields:
    id            - stable slug, shown in violations and used to opt out.
    term_hex      - the banned term, lowercase hex (never plaintext).
    repos         - list of repo slugs (origin basename) the rule fires in;
                    omit or None to fire everywhere.
    allow_globs   - repo-relative path globs where the term is permitted.
    only_globs    - if set, the rule fires *only* in these paths (the dual of
                    allow_globs; e.g. enforce on the front-page README alone).
    word_boundary - match on \\b...\\b (default True; `ward` skips `forward`).
    case_sensitive- default False.
    message       - remediation, printed on a hit. Never name the term here.

The rules below cover the three leak/coupling classes (sensitive data,
private->public reference, dependency cycle); the dependency-cycle class carries
more than one edge. See docs/leak-guard.md.
"""
from __future__ import annotations

RULES: list[dict] = [
    # Sensitive data: an employer name should never be grep-bait. Public bio
    # surfaces are allowlisted pending a decode-at-build step (docs/leak-guard).
    {
        "id": "employer-name",
        "term_hex": "6b617077696e67",
        "repos": None,
        "allow_globs": [
            "**/resume.md",
            "**/service_history.md",
            "docs/substrate.md",
            "data/repo-digests/**",
        ],
        "message": (
            "employer name is grep-bait here. Resolve it at run time from SSM / "
            "$AOS_HOST_CLASS (see infrastructure agent-compose), or for a public "
            "bio surface migrate to the decode-at-build step and allowlist it."
        ),
    },
    # Sensitive data: an opaque tailnet suffix identifies a private host and is
    # grep-bait in tracked config - dial via a run-time env var.
    {
        "id": "tailnet-suffix-tower",
        "term_hex": "7461696c303961343162",
        "repos": None,
        "allow_globs": [],
        "message": (
            "an opaque tailnet suffix is grep-bait in tracked content. Dial the "
            "host through a run-time env var (e.g. ${OLLAMA_BASE_URL}) resolved "
            "from SSM (/coilysiren/<repo>/read/<tail>), or use a <PLACEHOLDER> in "
            "docs - never the literal tailnet FQDN."
        ),
    },
    # Private -> public leak: the public front-page README must not name the
    # private bridge repo (internal tooling may enumerate siblings everywhere).
    {
        "id": "bridge-in-public-readme",
        "term_hex": "6167656e7469632d6f732d6b6169",
        "repos": ["agentic-os"],
        "only_globs": ["README.md"],
        "message": (
            "public README front-page names a private bridge repo. Describe the "
            "overlay generically (a private overlay repo); internal tooling/docs "
            "may still enumerate siblings."
        ),
    },
    # Dependency cycle: a public skill naming LUCA (private substrate consuming
    # this repo's hooks) closes the cycle.
    {
        "id": "luca-in-public-skills",
        "term_hex": "6c756361",
        "repos": ["agentic-os"],
        "only_globs": [".agents/skills/**", ".agents/composed/**"],
        "allow_globs": [],
        "message": (
            "public agentic-os skill names LUCA, a private observability "
            "substrate that consumes this repo's hooks - a dependency cycle. "
            "Keep OTel/observability content generic here and move LUCA-specific "
            "detail into a private overlay surface."
        ),
    },
    # Dependency cycle: cli-guard and ward reference each other. Break the cycle
    # by banning the cli-guard -> ward direction (cli-guard is the lower layer).
    {
        "id": "cli-guard-ward-cycle",
        "term_hex": "77617264",
        "repos": ["cli-guard"],
        "allow_globs": [],
        "message": (
            "cli-guard references ward, closing a dependency cycle (ward already "
            "depends on cli-guard). Drop the back-reference so the edge stays "
            "one-way."
        ),
    },
]
