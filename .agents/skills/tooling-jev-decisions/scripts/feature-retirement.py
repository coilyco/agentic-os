#!/usr/bin/env python3
"""Ask Jev, per FEATURES.md entry, whether each feature is worth its upkeep.

Usage: feature-retirement.py ROOT OUT [--skip REPO ...]

ROOT holds repository checkouts, each read at <repo>/docs/FEATURES.md. OUT is a
run directory: answers append to OUT/answers.jsonl as they land, so a rerun
skips what already answered, and OUT/ranked.json sorts by P(retire)+P(shrink).
JEV_PROXY_URL is the Agent Proxy base URL serving /v1/systemone.
"""
import argparse
import json
import os
import re
import sys
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

MODEL = "jev-1.13.0"
WIDTH = 16

# Symmetric on purpose: a state that names only upkeep reads as a retire prompt.
FLEET_FACTS = (
    "One operator owns every repository in this fleet. Most code is written and maintained by AI agents "
    "under that operator's direction. Every shipped feature carries ongoing upkeep: tests, docs, release "
    "payload, review attention, and breakage when its dependencies move. Every removal carries a cost too: "
    "whatever the feature does for its users or for other repositories stops, and restoring it later costs rework."
)
LEVELS = [
    "retire: remove the feature entirely; what it does is not worth its upkeep",
    "shrink: keep a reduced form; part of it is not worth its upkeep",
    "keep: worth its upkeep as it stands",
    "core: the repository's purpose depends on it",
]
QUESTIONS = {
    "valuable": {"type": "noul", "instructions":
                 "Would removing this feature lose value that its users, the operator, or other repositories "
                 "actually rely on? Judge only from state."},
    "disposition": {"type": "score", "criteria": LEVELS, "instructions":
                    "Rate the right disposition for this feature, weighing its upkeep against what removal "
                    "would lose. Judge only from state."},
}


def repo_purpose(repo: Path) -> str:
    readme = repo / "README.md"
    if not readme.exists():
        return ""
    for para in readme.read_text(errors="ignore").split("\n\n"):
        para = para.strip()
        if para and not para.startswith(("#", "!", "[", "<", "```", "|")):
            return re.sub(r"\s+", " ", para)[:600]
    return ""


def parse_features(md: str) -> list[dict]:
    out, section, cur = [], "", None
    for line in md.splitlines():
        if line.startswith("## "):
            section, cur = line[3:].strip(), None
        elif m := re.match(r"^[-*] (.+)", line):
            cur = {"section": section, "text": m.group(1).strip()}
            out.append(cur)
        elif cur is not None and line.startswith(("  ", "\t")) and line.strip():
            cur["text"] += " " + line.strip()
        elif not line.strip():
            cur = None
    for f in out:
        bold = re.match(r"\*\*([^*]+)\*\*", f["text"])
        f["name"] = (bold.group(1) if bold else f["text"][:60]).strip(" -")
    # A bare "See also" link line is navigation, not a feature.
    return [f for f in out if len(f["text"]) > 25 and not re.fullmatch(r"\[[^\]]+\]\([^)]+\)\s*-?.{0,60}", f["text"])]


def ask(proxy: str, body: dict) -> dict:
    req = urllib.request.Request(proxy.rstrip("/") + "/v1/systemone", json.dumps(body).encode(),
                                 {"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=120) as r:
        return json.load(r)


def rank(rows: list[dict]) -> list[dict]:
    out = []
    for r in rows:
        a = r["answer"].get("answers")
        if not a:
            continue
        p = {str(k): v for k, v in a["disposition"].get("probabilities", {}).items()}
        out.append({"id": r["id"], "repo": r["repo"], "name": r["name"], "valuable": a["valuable"]["noul"],
                    "retire": p.get("0", 0), "shrink": p.get("1", 0), "keep": p.get("2", 0), "core": p.get("3", 0),
                    "confidence": a["disposition"]["confidence"]})
    return sorted(out, key=lambda x: -(x["retire"] + x["shrink"]))


def main() -> None:
    ap = argparse.ArgumentParser(description="Ask Jev whether each FEATURES.md entry is worth its upkeep.")
    ap.add_argument("root", type=Path)
    ap.add_argument("out", type=Path)
    ap.add_argument("--skip", nargs="*", default=[], help="repository directory names to leave out")
    a = ap.parse_args()
    proxy = os.environ.get("JEV_PROXY_URL") or sys.exit("JEV_PROXY_URL is not set")
    a.out.mkdir(parents=True, exist_ok=True)

    items = []
    for repo in sorted(p for p in a.root.iterdir() if p.is_dir()):
        page = repo / "docs" / "FEATURES.md"
        if repo.name in a.skip or not page.exists():
            continue
        purpose = repo_purpose(repo)
        for i, f in enumerate(parse_features(page.read_text(errors="ignore"))):
            items.append({"id": f"{repo.name}#{i:02d}", "repo": repo.name, "purpose": purpose, **f})

    answers = a.out / "answers.jsonl"
    done = {}
    if answers.exists():
        for line in answers.read_text().splitlines():
            rec = json.loads(line)
            done[rec["id"]] = rec
    todo = [it for it in items if "error" in done.get(it["id"], {"answer": {"error": 1}})["answer"]]
    with ThreadPoolExecutor(WIDTH) as ex, answers.open("a") as fh:
        futs = {ex.submit(ask, proxy, {"model": MODEL, "questions": QUESTIONS, "state": {
            "fleet": FLEET_FACTS, "repository": it["repo"], "repository_purpose": it["purpose"],
            "feature_section": it["section"], "feature_description": it["text"]}}): it for it in todo}
        for fut in as_completed(futs):
            it = futs[fut]
            try:
                ans = fut.result()
            except Exception as e:  # recorded, so the rerun retries it
                ans = {"error": repr(e)}
            done[it["id"]] = {**it, "answer": ans}
            fh.write(json.dumps(done[it["id"]]) + "\n")

    ranked = rank([done[it["id"]] for it in items if it["id"] in done])
    (a.out / "ranked.json").write_text(json.dumps(ranked, indent=1))
    errs = sum(1 for it in items if "error" in done[it["id"]]["answer"])
    print(f"features {len(items)}, answered {len(items) - errs}, ranked -> {a.out / 'ranked.json'}")
    for x in ranked[:15]:
        print(f"{x['retire']:.2f} {x['shrink']:.2f} conf={x['confidence']:.2f}  {x['repo']} // {x['name'][:60]}")


if __name__ == "__main__":
    main()
