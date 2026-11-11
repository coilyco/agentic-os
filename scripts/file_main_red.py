#!/usr/bin/env python3
"""File a red main run on the Linear tracker, one open record per workflow (COI-3606).

A Sentry event reaches no one who works the queue, so a red main run becomes a record
where a seat looks. A second red run for the same workflow comments on the open record
instead of opening another. The key arrives by environment, never argv. Without it the
step warns and exits 0, so landing this before the secret exists breaks nothing.
"""

from __future__ import annotations

import json
import os
import sys
import urllib.request

API_URL = os.environ.get("LINEAR_API_URL", "https://api.linear.app/graphql")
TEAM_KEY = "COI"
LABELS = ("agentic-os", "role:eng-platform", "Bug")
OPEN_STATES = ["triage", "backlog", "unstarted", "started"]


def call(key: str, query: str, variables: dict) -> dict:
    """One GraphQL request, returning its data or raising on any error."""
    body = json.dumps({"query": query, "variables": variables}).encode()
    headers = {"Authorization": key, "Content-Type": "application/json"}
    request = urllib.request.Request(API_URL, data=body, headers=headers)
    with urllib.request.urlopen(request, timeout=20) as response:
        payload = json.load(response)
    if payload.get("errors"):
        raise RuntimeError(json.dumps(payload["errors"])[:300])
    return payload["data"]


def file_failure(key: str, title: str, body: str) -> str:
    """Comment on the open record with this title, or open one. Returns its identifier."""
    found = call(
        key,
        """query($title: String!, $team: String!, $states: [String!]!) {
          issues(first: 1, filter: {
            title: {eq: $title}, team: {key: {eq: $team}}, state: {type: {in: $states}}
          }) { nodes { id identifier } }
        }""",
        {"title": title, "team": TEAM_KEY, "states": OPEN_STATES},
    )["issues"]["nodes"]
    if found:
        call(
            key,
            "mutation($issue: String!, $body: String!) { commentCreate(input: {issueId: $issue, body: $body}) { success } }",
            {"issue": found[0]["id"], "body": body},
        )
        return found[0]["identifier"]
    # Linear rejects a query over 10000 complexity, so ask for the three labels by name.
    team = call(
        key,
        """query($team: String!, $names: [String!]!) {
          teams(filter: {key: {eq: $team}}) { nodes { id labels(filter: {name: {in: $names}}) { nodes { id name } } } }
        }""",
        {"team": TEAM_KEY, "names": list(LABELS)},
    )["teams"]["nodes"][0]
    labels = [label["id"] for label in team["labels"]["nodes"] if label["name"] in LABELS]
    created = call(
        key,
        "mutation($input: IssueCreateInput!) { issueCreate(input: $input) { issue { identifier } } }",
        {"input": {"teamId": team["id"], "title": title, "description": body, "labelIds": labels, "priority": 2}},
    )
    return created["issueCreate"]["issue"]["identifier"]


def describe(env: dict) -> tuple[str, str]:
    """The record title and body for the run these GITHUB_* variables name."""
    repo = env.get("GITHUB_REPOSITORY", "unknown repo")
    workflow = env.get("GITHUB_WORKFLOW", "unknown workflow")
    sha = env.get("GITHUB_SHA", "")
    run = f"{env.get('GITHUB_SERVER_URL', 'https://github.com')}/{repo}/actions/runs/{env.get('GITHUB_RUN_ID', '')}"
    subject = (env.get("COMMIT_SUBJECT") or "").strip().splitlines()[:1]
    lines = [
        f"The `{workflow}` workflow is red on `main` of `{repo}`.",
        "",
        f"* Run - {run}",
        f"* Commit - `{sha[:8]}` {subject[0] if subject else ''}".rstrip(),
        "",
        "Every main commit gets its own run, so this verdict is for that commit alone.",
        "Look at the run before the next release batch ships from main. A later red run",
        "for this workflow comments here until this record is closed.",
    ]
    return f"{repo} main: {workflow} is red", "\n".join(lines)


def main() -> int:
    key = os.environ.get("LINEAR_ALERT_KEY", "").strip()
    title, body = describe(dict(os.environ))
    if not key:
        print(f"::warning::LINEAR_ALERT_KEY is not set, so no record was filed for: {title}")
        return 0
    print(file_failure(key, title, body))
    return 0


if __name__ == "__main__":
    sys.exit(main())
