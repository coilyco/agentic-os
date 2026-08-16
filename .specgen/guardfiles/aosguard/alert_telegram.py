#!/usr/bin/env python3
"""Send a CI or CD failure alert through the in-cluster signoz-telegram mapper.

Embedded into aosguard and sealed, so a workflow step is one verb and no
repository carries a copy of this file. Stdlib-only: it runs under `python3 -I`
with no package available to import.

The mapper holds the Telegram identity in pod environment, so nothing here and
no caller needs a bot token. Both clusters serve it under the same cluster-local
name, so a job reaches its own cluster's instance.

Every field comes from the runner's own GITHUB_* variables. An explicit value
still wins, which is what lets a caller mark a deploy job as CD.
"""

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request

DEFAULT_ALERT_URL = (
    "http://signoz-telegram.signoz-telegram.svc.cluster.local:8080/api/create_alert"
)

# GITHUB_SERVER_URL is the cluster-local name the runner registered against, so
# a link built from it is unreachable from a phone. This is the forge ROOT_URL.
DEFAULT_FORGE_URL = "https://forgejo.coilysiren.me"


def field(name: str, *runner_vars: str) -> str:
    """An explicit override, else the runner's own value, else a visible gap.

    A missing field must never cost the alert. "workflow: ?" still says
    something broke.
    """
    for var in (name, *runner_vars):
        value = os.environ.get(var, "").strip()
        if value:
            return value
    return "?"


def run_url() -> str:
    explicit = os.environ.get("RUN_URL", "").strip()
    if explicit:
        return explicit
    base = os.environ.get("FORGE_URL", "").strip() or DEFAULT_FORGE_URL
    repo = os.environ.get("REPO", "") or os.environ.get("GITHUB_REPOSITORY", "")
    run_id = os.environ.get("GITHUB_RUN_ID", "")
    if not (repo and run_id):
        return "?"
    return f"{base.rstrip('/')}/{repo}/actions/runs/{run_id}"


def build_message() -> str:
    repo = field("REPO", "GITHUB_REPOSITORY")
    kind = os.environ.get("ALERT_KIND", "").strip() or "CI"
    return "\n".join(
        [
            f"{repo} {kind} failing",
            f"workflow: {field('WORKFLOW', 'GITHUB_WORKFLOW')}",
            f"run: {run_url()}",
        ]
    )


def main() -> int:
    url = os.environ.get("ALERT_URL", "").strip() or DEFAULT_ALERT_URL
    request = urllib.request.Request(
        url,
        data=json.dumps({"text": build_message()}).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    # The mapper is in-cluster. FORGEJO_EGRESS_PROXY is for external egress and
    # would send this the wrong way, so proxies are disabled explicitly.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(request, timeout=15) as response:
            print(f"alert posted: {response.status}")
    except urllib.error.URLError as exc:
        print(f"alert failed ({type(exc).__name__})", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
