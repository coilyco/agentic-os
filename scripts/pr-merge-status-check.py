#!/usr/bin/env python3
"""Stop hook: verify PR merge-status claims in the reply against Forgejo.

Finds each `repo!N` reference sitting near merge vocabulary and asks Forgejo
what is true. Scoped to `!N`; a bare `repo#N` is usually a tracker record.

AOS_PR_MERGE_MODE: off, warn (default, log only), block (rewrite on a
contradiction). Fails open throughout, so an unreachable API never strands a
turn. Listed in docs/FEATURES.md.
"""
import json
import os
import re
import subprocess
import sys
from datetime import datetime, timezone

MODE = os.environ.get("AOS_PR_MERGE_MODE", "warn")
LOG = os.environ.get(
    "AOS_PR_MERGE_LOG", os.path.expanduser("~/.claude/pr-merge-status.log")
)
FORGEJO = "https://forgejo.coilysiren.me"
TOKEN_PARAM = "/forgejo/coilyco-ops/api-token"

# Repos whose org is already known. Anything else is probed against each org.
ORGS = ("coilyco-flight-deck", "coilyco-bridge", "coilyco-gaming", "coilysiren")
KNOWN = {
    "agentic-os": "coilyco-flight-deck", "agent-proxy": "coilyco-flight-deck",
    "umbra": "coilyco-flight-deck", "agent-compose": "coilyco-flight-deck",
    "housecast": "coilyco-flight-deck", "mcp-beaver": "coilyco-flight-deck",
    "deploy": "coilyco-bridge", "infrastructure": "coilyco-bridge",
    "agentic-os-kai": "coilyco-bridge", "lore": "coilyco-bridge",
    "sirens-echo": "coilyco-gaming", "tally": "coilyco-gaming",
}

REF = re.compile(r"(?:([a-z0-9-]+)/)?([a-z0-9][a-z0-9-]*)!(\d+)")
MERGE_WORDS = re.compile(
    r"\b(merged|unmerged|merge[sd]?\b|not merged|still open|awaiting merge|"
    r"pending merge|has not landed|landed|not applied)\b", re.I)
NEGATIVE = re.compile(
    r"\b(not merged|unmerged|still open|has not merged|hasn't merged|"
    r"not been merged|awaiting merge|pending merge|not landed|has not landed)\b",
    re.I)


def log(line):
    try:
        os.makedirs(os.path.dirname(LOG), exist_ok=True)
        stamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        with open(LOG, "a", encoding="utf-8") as handle:
            handle.write(f"{stamp} {line}\n")
    except OSError:
        pass


def token():
    try:
        out = subprocess.run(
            ["aosguard", "ops", "aws", "ssm", "get-parameter", "--name", TOKEN_PARAM,
             "--with-decryption", "--query", "Parameter.Value", "--output", "text"],
            capture_output=True, text=True, timeout=20, check=False)
        return out.stdout.strip() or None
    except (OSError, subprocess.SubprocessError):
        return None


def fetch(org, repo, number, auth):
    """Read one pull request. curl rather than urllib: this host's python3 cannot
    verify the Forgejo certificate chain, which is agentic-os#25."""
    url = f"{FORGEJO}/api/v1/repos/{org}/{repo}/pulls/{number}"
    command = ["curl", "-sS", "-m", "10", url]
    if auth:
        command += ["-H", f"Authorization: token {auth}"]
    try:
        out = subprocess.run(command, capture_output=True, text=True,
                             timeout=15, check=False)
        body = json.loads(out.stdout or "{}")
    except (OSError, subprocess.SubprocessError, ValueError):
        return None
    if not isinstance(body, dict):
        return None
    # A private repo answers 200 with nulls when the token cannot see it.
    if body.get("state") is None:
        return None
    return {"merged": bool(body.get("merged")), "state": body.get("state"),
            "title": body.get("title") or "", "url": body.get("html_url") or url}


def main():
    if MODE == "off":
        return 0
    try:
        payload = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        return 0
    if payload.get("stop_hook_active"):
        return 0

    message = payload.get("last_assistant_message") or ""
    if not message or not MERGE_WORDS.search(message):
        return 0

    seen, findings = set(), []
    auth = None
    for match in REF.finditer(message):
        org, repo, number = match.group(1), match.group(2), match.group(3)
        window = message[max(0, match.start() - 150): match.end() + 150]
        if not MERGE_WORDS.search(window):
            continue
        key = (org or "", repo, number)
        if key in seen:
            continue
        seen.add(key)

        if auth is None:
            auth = token() or ""
        candidates = [org] if org else [KNOWN.get(repo)] if repo in KNOWN else list(ORGS)
        state = None
        for candidate in [c for c in candidates if c] or list(ORGS):
            state = fetch(candidate, repo, number, auth)
            if state:
                org = candidate
                break
        if not state:
            log(f"unresolved ref={repo}!{number} (private repo, bad token, or no such PR)")
            continue

        claimed_not_merged = bool(NEGATIVE.search(window))
        contradiction = claimed_not_merged == state["merged"]
        findings.append({
            "ref": f"{org}/{repo}!{number}", "merged": state["merged"],
            "state": state["state"], "url": state["url"],
            "contradiction": contradiction,
        })
        log(f"ref={org}/{repo}!{number} merged={state['merged']} "
            f"state={state['state']} contradiction={contradiction}")

    if not findings:
        return 0

    conflicts = [f for f in findings if f["contradiction"]]
    if conflicts and MODE == "block":
        lines = [
            f"  {f['ref']}: Forgejo says merged={f['merged']}, state={f['state']} ({f['url']})"
            for f in conflicts
        ]
        print(json.dumps({
            "decision": "block",
            "reason": (
                "You stated a merge status that Forgejo contradicts. Correct the "
                "reply against what the API actually returned:\n" + "\n".join(lines) +
                "\n\nMerged is not the same as applied or deployed. If you meant "
                "applied, say applied and check that separately."
            ),
        }))
        return 0

    facts = "; ".join(
        f"{f['ref']} merged={f['merged']} state={f['state']}" for f in findings)
    print(json.dumps({
        "systemMessage": f"PR merge status verified against Forgejo: {facts}",
        "suppressOutput": True,
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())
