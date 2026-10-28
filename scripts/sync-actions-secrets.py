#!/usr/bin/env python3
"""Sync Actions secrets from their SSM sources of truth to Forgejo or GitHub.

Actions secrets are write-only on both forges, so drift shows up as a silently dead
alert, publication, or auto-deploy step. Entries are keyed `owner/repo`. Values never
touch disk or argv: read from SSM with the AWS CLI, then PUT to the Forgejo API or
piped to `gh secret set`, which seals them. `--check` lists GitHub secret names only.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request

from agentic_os import shared_ssl_context

FORGEJO_BASE = "https://forgejo.coilysiren.me/api/v1"
OWNER = "coilyco"


def slug(repo: str, owner: str = OWNER) -> str:
    """Return the ``owner/repo`` key the Forgejo secrets API addresses."""
    return f"{owner}/{repo}"


# Sources that name no forge credential, so both targets carry them.
# The aterm Android release keystore, read by aterm-android.yml (COI-2623).
ATERM_ANDROID_SECRETS = {
    "ATERM_ANDROID_KEYSTORE_B64": "/coilysiren/aterm/write/android-keystore-b64",
    "ATERM_ANDROID_KEYSTORE_PASSWORD": "/coilysiren/aterm/write/android-keystore-password",
    "ATERM_ANDROID_CERT_SHA256": "/coilysiren/aterm/write/android-keystore-cert-sha256",
}
# housecast's PyPI train, read by its publish workflow. Trusted publishing
# has no Forgejo identity provider, so an API token is the only path.
PYPI_SECRETS = {"PYPI_TOKEN": "/coilysiren/pypi/token"}

# owner/repo -> secret name -> SSM parameter (see SSM.md in agentic-os-kai), the
# owner-qualified key letting one mapping span orgs. Writers rotate separately.
MAPPING: dict[str, dict[str, str]] = {
    slug("agentic-os"): {
        "CI_RELEASE_TOKEN": "/forgejo/coilyco-ops/ci-release-token",
        "TAP_WRITE_TOKEN": "/forgejo/coilyco-ops/tap-bump-token",
        "SCOOP_WRITE_TOKEN": "/forgejo/coilyco-ops/scoop-write-token",
        **ATERM_ANDROID_SECRETS,
    },
    slug("umbra"): {
        "CI_RELEASE_TOKEN": "/forgejo/coilyco-ops/ci-release-token",
    },
    slug("housecast"): dict(PYPI_SECRETS),
    # deploy's scheduled pin reconciler.
    slug("deploy"): {
        "DEPLOY_PUSH_TOKEN": "/forgejo/coilyco-ops/ci-release-token",
        "REGISTRY_READ_TOKEN": "/forgejo/coilyco-ops/registry-read-token",
    },
}

# GitHub target (COI-2936): never the Forgejo PATs above. Workflows get the coilyco-ops
# App id and base64 key and mint installation tokens. See docs/aos-cli.md.
APP_SECRETS = {
    "COILYCO_OPS_APP_ID": "/github/coilyco-ops-app-id",
    "COILYCO_OPS_APP_PRIVATE_KEY_BASE64": "/github/coilyco-ops-private-key-base64",
}
TANGLED_SECRETS = {"TANGLED_SSH_KEY": "/coilysiren/tangled/push-key"}
GITHUB_MAPPING: dict[str, dict[str, str]] = {
    # The tap, scoop and release-push sources in the COI-2935 consumer map. The tap
    # and bucket repos are only mint targets, so they hold no secret.
    slug("agentic-os"): {**APP_SECRETS, **ATERM_ANDROID_SECRETS},
    slug("umbra"): {**APP_SECRETS, **TANGLED_SECRETS},
    slug("agent-compose"): {**APP_SECRETS, **TANGLED_SECRETS},
    slug("mcp-beaver"): dict(TANGLED_SECRETS),
    slug("housecast"): dict(PYPI_SECRETS),
}
# Forgejo reads this from an org secret. GitHub's org-secret plan tier is
# unverified, so the GitHub target sets it per repo instead.
FLEET_SECRETS = {"SENTRY_CI_DSN": "/coilysiren/sentry/dsn/fleet-heartbeats"}
# Repos carrying `.forgejo/workflows`, read from the Forgejo API 2026-10-27 (COI-2928).
# A repo leaves this tuple when its Forgejo copy is archived.
WORKFLOW_REPOS = tuple(
    slug(repo)
    for repo in (
        "agent-compose", "agent-proxy", "agentic-os", "agentic-os-hardware",
        "agentic-os-kai", "agentic-os-xxx", "big-type", "bluesky-mcp", "deploy",
        "eco-app", "eco-mods", "eco-ops", "enshrouded", "factorio-mods",
        "factory-game-v3", "galaxy-gen", "homebrew-tap", "housecast",
        "infrastructure", "lore", "lunch-money-k8s", "mcp-beaver",
        "node-stats-mcp", "scoop-bucket", "sentry-bridge", "sirens-echo",
        "ssm-app", "steam-ops", "tally", "umbra", "voice-corpus", "website",
    )
)

# Mirrors Forgejo's own models/secret ValidateName, so a bad name fails here at
# plan time instead of as a mid-write HTTP 400 that half-applies the plan.
SECRET_NAME_PATTERN = re.compile("(?i)^[A-Z_][A-Z0-9_]*$")
FORBIDDEN_SECRET_PREFIX = re.compile("(?i)^(FORGEJO_|GITEA_|GITHUB_|[0-9])")
GITHUB_FORBIDDEN_SECRET_PREFIX = re.compile("(?i)^(GITHUB_|[0-9])")


def invalid_secret_names(
    mapping: dict[str, dict[str, str]],
    forbidden: re.Pattern[str] = FORBIDDEN_SECRET_PREFIX,
) -> list[str]:
    """Return `repo_slug: NAME` for every secret the target forge would reject."""
    return [
        f"{repo_slug}: {name}"
        for repo_slug, secrets in mapping.items()
        for name in secrets
        if not SECRET_NAME_PATTERN.match(name) or forbidden.match(name)
    ]


def build_plan(
    mapping: dict[str, dict[str, str]],
    fleet: dict[str, str] | None = None,
    repos: tuple[str, ...] = (),
) -> list[tuple[str, str, str]]:
    """Return (repo, name, ssm param) rows: fleet secrets on every repo, a repo's own win."""
    merged = {repo_slug: dict(fleet or {}) for repo_slug in repos}
    for repo_slug, secrets in mapping.items():
        merged.setdefault(repo_slug, {}).update(secrets)
    return [
        (repo_slug, name, param)
        for repo_slug, secrets in merged.items()
        for name, param in secrets.items()
    ]


def ssm_get(name: str) -> str:
    out = subprocess.run(
        [
            "aws", "ssm", "get-parameter", "--name", name, "--with-decryption",
            "--query", "Parameter.Value", "--output", "text",
        ],
        check=True,
        capture_output=True,
        text=True,
    )
    value = out.stdout.strip()
    if not value:
        raise SystemExit(f"ssm parameter {name} resolved empty")
    # OpenSSH rejects a private key with no final newline.
    return value + "\n" if value.startswith("-----BEGIN") else value


def put_secret(token: str, repo_slug: str, name: str, value: str) -> None:
    url = f"{FORGEJO_BASE}/repos/{repo_slug}/actions/secrets/{name}"
    body = json.dumps({"data": value}).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=body,
        method="PUT",
        headers={
            "Authorization": f"token {token}",
            "Content-Type": "application/json",
        },
    )
    with urllib.request.urlopen(
        request, timeout=30, context=shared_ssl_context()
    ) as response:
        response.read()


def gh_set_secret(_token: str, repo_slug: str, name: str, value: str) -> None:
    """Seal and set one repo secret; the value rides stdin, so it never reaches argv."""
    subprocess.run(
        ["gh", "secret", "set", name, "--repo", repo_slug],
        input=value,
        text=True,
        check=True,
        capture_output=True,
    )


def gh_secret_names(repo_slug: str) -> set[str]:
    """Return the repo secret names GitHub holds. Values are never readable."""
    out = subprocess.run(
        ["gh", "secret", "list", "--repo", repo_slug, "--json", "name", "--jq", ".[].name"],
        check=True,
        capture_output=True,
        text=True,
    )
    return set(out.stdout.split())


def check_plan(plan: list[tuple[str, str, str]], lister=gh_secret_names) -> tuple[list[str], int]:
    """Return one report line per repo and how many repos are missing a name or unreadable."""
    wanted: dict[str, list[str]] = {}
    for repo_slug, name, _param in plan:
        wanted.setdefault(repo_slug, []).append(name)
    lines: list[str] = []
    problems = 0
    for repo_slug, names in wanted.items():
        try:
            present = lister(repo_slug)
        except (subprocess.CalledProcessError, OSError) as err:
            lines.append(f"{repo_slug}: unreadable ({type(err).__name__})")
            problems += 1
            continue
        missing = [name for name in names if name not in present]
        if missing:
            lines.append(f"{repo_slug}: missing {', '.join(missing)}")
            problems += 1
        else:
            lines.append(f"{repo_slug}: ok ({len(names)})")
    return lines, problems


def admin_token() -> str:
    """Return the attended site-admin token supplied by the operator."""
    token = os.environ.get("FORGEJO_ADMIN_TOKEN", "").strip()
    if not token:
        raise SystemExit(
            "FORGEJO_ADMIN_TOKEN is required; load it with the attended "
            "infrastructure forgejo-admin-token helper"
        )
    return token


def apply_plan(
    plan: list[tuple[str, str, str]],
    token: str,
    read=ssm_get,
    write=put_secret,
) -> list[str]:
    """Write every secret, returning one line per failure instead of stopping.

    A stale repo or an empty SSM parameter must not leave the entries after it
    unsynced (COI-2634). A failure names the repo, secret and cause, never a value.
    """
    failures: list[str] = []
    for repo_slug, name, param in plan:
        try:
            write(token, repo_slug, name, read(param))
        except urllib.error.HTTPError as err:
            failures.append(f"{repo_slug}: {name}: HTTP {err.code}")
        except (urllib.error.URLError, subprocess.CalledProcessError, OSError, SystemExit) as err:
            failures.append(f"{repo_slug}: {name}: {type(err).__name__}")
        else:
            print(f"{repo_slug}: {name} set")
    return failures


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--target",
        choices=("forgejo", "github"),
        default="forgejo",
        help="forge to write to (default forgejo until the COI-2927 cutover)",
    )
    parser.add_argument(
        "--repo",
        action="append",
        default=[],
        metavar="OWNER/REPO",
        help="limit the plan to this repo, repeatable. One installation token covers one repo",
    )
    parser.add_argument(
        "--secret",
        action="append",
        default=[],
        metavar="NAME",
        help="limit the plan to this secret name, repeatable",
    )
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--dry-run", action="store_true", help="print the plan, write nothing")
    mode.add_argument(
        "--check",
        action="store_true",
        help="github only: list which planned secret names each repo holds, names only",
    )
    args = parser.parse_args(argv)
    github = args.target == "github"
    if args.check and not github:
        raise SystemExit("--check reads GitHub secret names; pass --target github")
    bad_repos = [repo for repo in args.repo if repo.count("/") != 1 or "" in repo.split("/")]
    if bad_repos:
        raise SystemExit("--repo takes owner/repo, got: " + ", ".join(bad_repos))

    def only(mapping: dict[str, dict[str, str]]) -> dict[str, dict[str, str]]:
        return {k: v for k, v in mapping.items() if not args.repo or k in args.repo}

    if github:
        github_mapping = only(GITHUB_MAPPING)
        plan = build_plan(github_mapping, FLEET_SECRETS, tuple(args.repo) or WORKFLOW_REPOS)
        named = {**github_mapping, "fleet": FLEET_SECRETS}
        forbidden, write, forge = GITHUB_FORBIDDEN_SECRET_PREFIX, gh_set_secret, "GitHub"
    else:
        named = only(MAPPING)
        plan = build_plan(named)
        forbidden, write, forge = FORBIDDEN_SECRET_PREFIX, put_secret, "Forgejo"

    # Before any write, so a rejected name cannot half-apply the plan.
    rejected = invalid_secret_names(named, forbidden)
    if rejected:
        raise SystemExit(
            f"{forge} would reject these secret names (reserved prefix or leading "
            "digit):\n  " + "\n  ".join(rejected)
        )

    if args.secret:
        plan = [row for row in plan if row[1] in args.secret]
        if not plan:
            raise SystemExit("--secret matched no planned secret: " + ", ".join(args.secret))

    if args.check:
        lines, problems = check_plan(plan)
        print("\n".join(lines))
        return 1 if problems else 0

    for repo_slug, name, param in plan:
        print(f"{repo_slug}: {name} <- ssm {param}")
    print(f"{len(plan)} secrets across {len({row[0] for row in plan})} repos on {forge}")
    if args.dry_run:
        return 0

    failures = apply_plan(plan, "" if github else admin_token(), write=write)
    if failures:
        print(f"{len(failures)} of {len(plan)} secrets failed:", file=sys.stderr)
        for line in failures:
            print(f"  {line}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
