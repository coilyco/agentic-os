# Coverage and counts

The gate on every triage pass. A classification over half a backlog is not a
partial answer, it is a wrong one, because the half you missed is invisible in
the output.

## Get the authoritative count first

Two endpoints return a total and they are backed by different sources:

- **Per-repo** `GET /api/v1/repos/{owner}/{repo}/issues?type=issues&state=open` - the `X-Total-Count` response header is a **direct DB query**. Authoritative. Sum this for accounting.
- **Cross-repo** `GET /api/v1/repos/issues/search?type=issues&state=open&owner={org}` - the `X-Total-Count` is served from Forgejo's **async issue indexer** (bleve by default), updated off a queue. Convenient for one org-wide number, eventually consistent.

Get the header with `curl -sD -`, because a `HEAD` or `curl -I` drops it:

```bash
TOKEN=$(aosguard ops aws ssm get-parameter --name /forgejo/coilyco-ops/read-token --with-decryption --query Parameter.Value --output text)
BASE=https://forgejo.coilysiren.me/api/v1
gettotal() { curl -sD - -o /dev/null -H "Authorization: token $TOKEN" "$1" | awk 'tolower($1)=="x-total-count:"{print $2}' | tr -d '\r'; }

# authoritative per-repo open-issue count
gettotal "$BASE/repos/coilyco-bridge/agentic-os-kai/issues?type=issues&state=open&limit=1"

# sum across an org
sum=0
for r in $(curl -s -H "Authorization: token $TOKEN" "$BASE/orgs/coilyco-bridge/repos?limit=100" | jq -r '.[].name'); do
  sum=$((sum + $(gettotal "$BASE/repos/coilyco-bridge/$r/issues?type=issues&state=open&limit=1")))
done
echo "$sum"
```

### It is indexer lag, not a per-repo offset

The 2026-05-29 burn-down saw the search total drift from the per-repo sum
(search said agentic-os-kai 374 against per-repo 373, and after closing 366
issues the search totals were off by tens). The original read, "search inflates
by about 1 per repo", is wrong. The mechanism is **indexer eventual
consistency**: the search index lags the DB, and the gap is widest right after a
bulk mutation, which is exactly when a burn-down reads it. It is not a fixed
offset, and waiting for the indexer to settle makes the search total converge
back to the DB.

Re-measured 2026-06-04 on Forgejo 15.0.2+gitea-1.22.0 with the indexer settled,
the two agreed exactly in every state (open 385, closed 525, all 910). The
upstream-bug suspicions from the original report were both ruled out: `type=pulls`
is counted separately rather than folded into `type=issues`, and pinned rows are
not double-counted. There is no Forgejo bug to file. The durable rule is only
that per-repo is DB-direct and cannot lag.

### The mandate covers every enumeration, not just the triage pass

The count gate is easy to read as a rule about the main fan-out. It is not. It
binds on **any** question of the form "which issues carry X", including the
quick ad-hoc scan you run to check your own work.

Measured twice in the 2026-09-01 session, both times by the author of this
page. The opening fleet count returned **739** against an authoritative **776**,
caught because the gate was applied. A later scan for one label returned
**21** against an authoritative **34**, not caught, because the gate was
skipped on a query that felt too small to need it. Acting on that number
relabelled 21 issues and silently left 13.

Both failures have the same shape: a page request fails, the loop reads the
empty result as the end of the collection, and the shortfall is invisible
because nothing declared what the total should have been.

### Filter server-side, and count the filter

Do not page a repository's whole issue list and filter by label in your own
code. Ask the API for the filtered set and read its total:

```bash
gettotal "$BASE/repos/$OWNER/$REPO/issues?type=issues&state=open&labels=priority%2FP4&limit=1"
```

The `labels=` filter takes URL-encoded label names, and the `X-Total-Count` on
that request is DB-direct for the filtered set exactly as it is for the whole
one. That turns a coverage problem into a single number, which is the only form
that can be checked. **Verify a bulk write by re-reading that number, not by
re-running the scan that produced the worklist.**

## The count mandate

When you fan discovery out one worker per repository, the workers reliably
**under-paginate**. They treat the first page or two as representative, stop at
roughly half, and report done. In the 2026-05-29 burn-down the first pass saw
800 of 1104 open issues (agentic-os-kai 235/373, infrastructure 88/140, coily
56/107), which would have left about 300 issues silently un-triaged.

"Page until empty" is not a forcing function. Agents stop early regardless. The
fix that worked is handing each worker its **exact expected open count** as a
hard coverage gate, baked into the prompt verbatim:

> Your repo `{owner}/{repo}` has exactly **N** open issues. Retrieve all N before you classify anything. Page (`page=1,2,3...`) until your accumulated set size equals N. If retrieved != N you have failed - keep paging or report the shortfall. Never triage a partial set.

## Spawn pattern

- **Count in the parent, first.** One header request per repo, cheap. Never delegate counting to the same agent that might under-fetch, because the mandate has to come from outside the agent being held to it.
- **One worker per repository, each carrying its own N.** Never a shared "go triage everything" prompt. A global mandate gives no per-repo check, which is exactly how the first pass drifted.
- **Real page size.** Pass `state=open&limit=50` and require paging until the accumulated count hits N, not until a page looks short.

## Verification gate

The pass is done only when `retrieved == N` for **every** repository. The parent
re-checks after the fan-out returns and does not trust a worker's self-report.
Any repo where retrieved < N is re-dispatched rather than waved through. In the
burn-down, the second pass with the count mandate matched the authoritative
count in every repo.

## Writes are classifier-gated, so drive mutations from the parent

Discovery fans out fine. **The write half** - applying labels, closing or
commenting on issues the agent did not author - reads to the auto-mode
classifier as a mass External System Write and hard-blocks regardless of
in-conversation authorization. The 2026-06-17 all-org cascade denied 4 of 14
label-writing workers even though the go-ahead had been given for all 40 repos.

Authorization does not propagate into a worker. Each is judged fresh, and an
earlier "dry-run first" answer in the parent's context reads as a standing wall
the worker inherits. Foreground parent writes pass when the go-ahead is
in-context.

So fan out the read and classify, then **drive mutations from the parent**, as a
loop in one Bash call or a scratchpad script, keeping label, close, and comment
calls out of worker prompts. The writes are reversible, so the gate is on the
form rather than the risk.

### The parent is not enough on its own: batch size counts too

Measured on the 2026-09-01 fleet audit. A parent-driven loop of **18** label
writes passed. A parent-driven loop of **130**, in the same session with the
same authorization and the same verb, was refused as a mass external-system
write. Nothing about the caller changed, only the count.

So "drive mutations from the parent" is necessary and not sufficient. Plan a
bulk relabel as **tens of writes, not hundreds**, scoped to a bucket you can
name in one sentence, and expect the refusal anyway on a fleet-sized pass.

**When it is refused, the reading is the thing worth saving.** Do not re-shape
the same loop to slip past the gate: that works around the intent rather than
the mechanism. File the classification as a tracker issue carrying one row per
issue with its evidence, so the pass is durable and someone with the right
surface can apply it. A refused write costs an hour. A lost reading of 188
issues costs the whole pass.

## See also

- [global-forgejo-scope](global-forgejo-scope.md) - the fleet inventory, distribution accounting, and completion rules for a whole-fleet pass.
- `/forgejo/coilyco-ops/read-token` in SSM - the scoped read PAT these calls use.
