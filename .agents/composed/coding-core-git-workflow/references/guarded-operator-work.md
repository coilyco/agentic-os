# AOSguard operator work

Use `aosguard ops <area> ...` for the operator families AOSguard exposes.
Enumerate a family with `aosguard ops <area> describe` or `--help` before
choosing a leaf. Ward owns repository development and agent dispatch, not a
parallel operator command tree.

Approved bare read commands remain available where the active harness policy
allows them. AOSguard's generated allowlist and the running role determine
write authority.

Shell metacharacters do not belong inside an AOSguard argument. Put any
approved pipe or redirect outside the guarded call.

## Disabling pull requests on a repo

GitHub's per-repo "disable pull requests" toggle (shipped Feb 2026) must be set via **GraphQL**, not REST. This is a sanctioned exception to the REST-default rule: the REST path is broken. `has_pull_requests` on `PATCH /repos/{owner}/{repo}` echoes whatever you send but does not persist, and the REST `GET` reads back stale/inverted values - do not trust it to confirm state. GraphQL `Repository.hasPullRequestsEnabled` is authoritative both to read and to write.

```graphql
# resolve the repo node id (and read current state)
query { repository(owner:"coilysiren",name:"REPO"){ id hasPullRequestsEnabled } }
# disable PRs
mutation { updateRepository(input:{repositoryId:"R_...",hasPullRequestsEnabled:false}){ repository{ name hasPullRequestsEnabled } } }
```

`pullRequestCreationPolicy` (`ALL` / `COLLABORATORS_ONLY`) is the softer "who
can open PRs" dropdown, also on `updateRepository`. The current AOSguard
surface does not expose GitHub GraphQL. An authorized GitHub-capable operator
must use the approved GitHub surface and pass the query from a file. Origin:
the REST behavior described above.
