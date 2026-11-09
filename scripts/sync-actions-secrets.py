#!/usr/bin/env python3
"""Sync Actions secrets from their SSM sources of truth to GitHub.

Actions secrets are write-only, so drift shows up as a silently dead alert,
publication, or auto-deploy step. Entries are keyed `owner/repo`. Values never touch
disk or argv: read from SSM with the AWS CLI, then piped to `gh secret set`, which
seals them. `--check` lists secret names only.
"""
from __future__ import annotations

import argparse
import base64
import re
import subprocess
import sys
from typing import NamedTuple

OWNER = "coilyco"


def slug(repo: str, owner: str = OWNER) -> str:
    """Return the ``owner/repo`` key `gh secret set --repo` addresses."""
    return f"{owner}/{repo}"


# The aterm Android release keystore, read by aterm-android.yml (COI-2623).
ATERM_ANDROID_SECRETS = {
    "ATERM_ANDROID_KEYSTORE_B64": "/coilysiren/aterm/write/android-keystore-b64",
    "ATERM_ANDROID_KEYSTORE_PASSWORD": "/coilysiren/aterm/write/android-keystore-password",
    "ATERM_ANDROID_CERT_SHA256": "/coilysiren/aterm/write/android-keystore-cert-sha256",
}
# The self-signed identity signing the macOS aterm bundle (COI-50). SSM keeps the
# aterm names until the alias retires (COI-3338), so the Actions names are final.
CODESIGN_SECRETS = {
    "TREEHOUSE_CODESIGN_CERT_PEM": "/coilysiren/codesign/aterm-cert",
    "TREEHOUSE_CODESIGN_KEY_PEM": "/coilysiren/codesign/aterm-key",
}
# housecast's PyPI train, read by its publish workflow.
PYPI_SECRETS = {"PYPI_TOKEN": "/coilysiren/pypi/token"}

# owner/repo -> secret name -> SSM parameter (see SSM.md in agentic-os-kai).
# Workflows mint installation tokens from the App id and key (COI-2936, docs/aos-cli.md).
APP_SECRETS = {
    "COILYCO_OPS_APP_ID": "/github/coilyco-ops-app-id",
    "COILYCO_OPS_APP_PRIVATE_KEY_BASE64": "/github/coilyco-ops-private-key-base64",
}
TANGLED_SECRETS = {"TANGLED_SSH_KEY": "/coilysiren/tangled/push-key"}
MAPPING: dict[str, dict[str, str]] = {
    # The tap, scoop and release-push sources in the COI-2935 consumer map. The tap
    # and bucket repos are only mint targets, so they hold no secret.
    slug("agentic-os"): {**APP_SECRETS, **ATERM_ANDROID_SECRETS, **CODESIGN_SECRETS},
    slug("umbra"): {**APP_SECRETS, **TANGLED_SECRETS},
    slug("agent-compose"): {**APP_SECRETS, **TANGLED_SECRETS},
    slug("mcp-beaver"): dict(TANGLED_SECRETS),
    slug("housecast"): dict(PYPI_SECRETS),
}


class OrgSecret(NamedTuple):
    """One org-level Actions secret with selected-repository visibility.

    `repos` are bare repo names, which is what `gh secret set --repos` takes.
    `encode_base64` is for a `_BASE64` secret whose SSM source is the raw value.
    A NamedTuple, because the test loaders import this file outside sys.modules
    and a dataclass looks itself up there.
    """

    ssm: str
    repos: tuple[str, ...]
    encode_base64: bool = False


# The coilyco-read App pair behind the private agent-compose fetch (COI-3302).
# Org-level only: a same-named repo secret would shadow it (COI-3382).
TREEHOUSE_READ_REPOS = ("agentic-os", "agentic-os-kai", "sirens-echo", "infrastructure")
ORG_SECRETS: dict[str, OrgSecret] = {
    "COILYCO_TREEHOUSE_READ_APP_ID": OrgSecret(
        "/coilysiren/github/coilyco-read-app/app-id", TREEHOUSE_READ_REPOS
    ),
    "COILYCO_TREEHOUSE_READ_APP_PRIVATE_KEY_BASE64": OrgSecret(
        "/coilysiren/github/coilyco-read-app/private-key", TREEHOUSE_READ_REPOS, encode_base64=True
    ),
}


def org_secret_names(repo_slug: str, org_secrets: dict[str, OrgSecret] | None = None) -> set[str]:
    """Return the org-level secret names this script syncs to `repo_slug`."""
    return {
        name
        for name, secret in (ORG_SECRETS if org_secrets is None else org_secrets).items()
        if repo_slug in {slug(repo) for repo in secret.repos}
    }


# GitHub's org-secret plan tier is unverified, so this is set per repo.
FLEET_SECRETS = {"SENTRY_CI_DSN": "/coilysiren/sentry/dsn/fleet-heartbeats"}
# Repos that carry workflows, read 2026-10-27 (COI-2928).
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

# GitHub's secret naming rules, so a bad name fails here at plan time instead of
# as a mid-write error that half-applies the plan.
SECRET_NAME_PATTERN = re.compile("(?i)^[A-Z_][A-Z0-9_]*$")
FORBIDDEN_SECRET_PREFIX = re.compile("(?i)^(GITHUB_|[0-9])")


def invalid_secret_names(
    mapping: dict[str, dict[str, str]],
    forbidden: re.Pattern[str] = FORBIDDEN_SECRET_PREFIX,
) -> list[str]:
    """Return `repo_slug: NAME` for every secret GitHub would reject."""
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


def gh_set_secret(repo_slug: str, name: str, value: str) -> None:
    """Seal and set one repo secret; the value rides stdin, so it never reaches argv."""
    subprocess.run(
        ["gh", "secret", "set", name, "--repo", repo_slug],
        input=value,
        text=True,
        check=True,
        capture_output=True,
    )


def encode_org_value(secret: OrgSecret, value: str) -> str:
    """Return the value to seal: base64 on one line when the secret name says so."""
    if not secret.encode_base64:
        return value
    return base64.b64encode(value.encode()).decode()


def gh_set_org_secret(name: str, secret: OrgSecret, value: str) -> None:
    """Seal and set one org secret on its selected repos; the value rides stdin."""
    subprocess.run(
        [
            "gh", "secret", "set", name, "--org", OWNER,
            "--visibility", "selected", "--repos", ",".join(secret.repos),
        ],
        input=value,
        text=True,
        check=True,
        capture_output=True,
    )


def gh_org_secret_names() -> set[str]:
    """Return the org secret names GitHub holds. Values are never readable."""
    out = subprocess.run(
        ["gh", "secret", "list", "--org", OWNER, "--json", "name", "--jq", ".[].name"],
        check=True,
        capture_output=True,
        text=True,
    )
    return set(out.stdout.split())


def check_org_plan(
    org_secrets: dict[str, OrgSecret], lister=gh_org_secret_names
) -> tuple[list[str], int]:
    """Return one report line per org secret name and how many are missing or unreadable."""
    try:
        present = lister()
    except (subprocess.CalledProcessError, OSError) as err:
        return [f"{OWNER} (org): unreadable ({type(err).__name__})"], 1
    lines = [
        f"{OWNER} (org): {name} " + ("ok" if name in present else "missing")
        for name in org_secrets
    ]
    return lines, sum(name not in present for name in org_secrets)


def apply_org_plan(
    org_secrets: dict[str, OrgSecret],
    read=ssm_get,
    write=gh_set_org_secret,
) -> list[str]:
    """Write every org secret, returning one line per failure and never a value."""
    failures: list[str] = []
    for name, secret in org_secrets.items():
        try:
            write(name, secret, encode_org_value(secret, read(secret.ssm)))
        except (subprocess.CalledProcessError, OSError, SystemExit) as err:
            failures.append(f"{OWNER} (org): {name}: {type(err).__name__}")
        else:
            print(f"{OWNER} (org): {name} set")
    return failures


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


def apply_plan(
    plan: list[tuple[str, str, str]],
    read=ssm_get,
    write=gh_set_secret,
) -> list[str]:
    """Write every secret, returning one line per failure instead of stopping.

    A stale repo or an empty SSM parameter must not leave the entries after it
    unsynced (COI-2634). A failure names the repo, secret and cause, never a value.
    """
    failures: list[str] = []
    for repo_slug, name, param in plan:
        try:
            write(repo_slug, name, read(param))
        except (subprocess.CalledProcessError, OSError, SystemExit) as err:
            failures.append(f"{repo_slug}: {name}: {type(err).__name__}")
        else:
            print(f"{repo_slug}: {name} set")
    return failures


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--target",
        choices=("github",),
        default="github",
        help="kept so existing invocations work; GitHub is the only target",
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
        help="list which planned secret names each repo holds, names only",
    )
    args = parser.parse_args(argv)
    bad_repos = [repo for repo in args.repo if repo.count("/") != 1 or "" in repo.split("/")]
    if bad_repos:
        raise SystemExit("--repo takes owner/repo, got: " + ", ".join(bad_repos))

    def only(mapping: dict[str, dict[str, str]]) -> dict[str, dict[str, str]]:
        return {k: v for k, v in mapping.items() if not args.repo or k in args.repo}

    mapping = only(MAPPING)
    # An org secret is not one repo's, so a --repo run leaves it alone.
    org_secrets = {} if args.repo else dict(ORG_SECRETS)
    plan = build_plan(mapping, FLEET_SECRETS, tuple(args.repo) or WORKFLOW_REPOS)
    named = {**mapping, "fleet": FLEET_SECRETS, "org": {n: s.ssm for n, s in org_secrets.items()}}

    # Before any write, so a rejected name cannot half-apply the plan.
    rejected = invalid_secret_names(named)
    if rejected:
        raise SystemExit(
            "GitHub would reject these secret names (reserved prefix or leading "
            "digit):\n  " + "\n  ".join(rejected)
        )

    if args.secret:
        plan = [row for row in plan if row[1] in args.secret]
        org_secrets = {n: s for n, s in org_secrets.items() if n in args.secret}
        if not plan and not org_secrets:
            raise SystemExit("--secret matched no planned secret: " + ", ".join(args.secret))

    if args.check:
        lines, problems = check_plan(plan)
        if org_secrets:
            org_lines, org_problems = check_org_plan(org_secrets)
            lines, problems = lines + org_lines, problems + org_problems
        print("\n".join(lines))
        return 1 if problems else 0

    for repo_slug, name, param in plan:
        print(f"{repo_slug}: {name} <- ssm {param}")
    for name, secret in org_secrets.items():
        print(f"{OWNER} (org, selected {len(secret.repos)} repos): {name} <- ssm {secret.ssm}")
    print(f"{len(plan)} secrets across {len({row[0] for row in plan})} repos on GitHub")
    if org_secrets:
        print(f"{len(org_secrets)} org secrets on GitHub")
    if args.dry_run:
        return 0

    failures = apply_plan(plan) + apply_org_plan(org_secrets)
    if failures:
        print(f"{len(failures)} of {len(plan)} secrets failed:", file=sys.stderr)
        for line in failures:
            print(f"  {line}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
