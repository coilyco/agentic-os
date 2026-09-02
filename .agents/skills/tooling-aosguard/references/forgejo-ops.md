# Forgejo ops and the GitHub mirror

Runner tokens and storage, the two operator reads that are not Actions.

## Forgejo runner-token fetch overlay

`aosguard ops forgejo actions generate-runner-token` mints a Forgejo Actions
runner registration token from AOSguard's standalone generated surface. Its
three scoped leaves are:

- `global` - `/admin/runners/registration-token`
- `org <org>` - `/orgs/{org}/actions/runners/registration-token`
- `repo <owner> <repo>` - `/repos/{owner}/{repo}/actions/runners/registration-token`

The generated AOSguard command owns request routing, authentication, and HTTP.
Ward does not consume this surface or mount its credentials.

## Forgejo storage measurement

`aosguard ops forgejo-storage measure` collects the application-aware storage
evidence needed after a general disk-pressure report identifies Forgejo as a
material owner.

## Authority boundary

The generic `aosguard ops kubectl` surface continues to deny `exec`. The
storage command is a separate sealed exec transport beside the Forgejo API
group. It accepts no caller arguments and fixes these details in embedded code:

* Namespace and application/database workload targets.
* Forgejo filesystem paths and bounded report depths.
* Every program and shell pipeline executed in the application workload.
* Every PostgreSQL statement executed in the database workload.

The command therefore cannot be widened into an arbitrary pod shell, path
reader, or SQL client by supplying another argument.

Specgen compiles the reviewed Python source into `aosguard`. At invocation it
materializes that source under a private temporary directory and gives Python
an absolute path. AOSguard never relies on a checkout-relative script path.

## Report

The command records the active Kubernetes context and PVC/pod ownership before
collecting independent sections for:

* Forgejo application root and managed data.
* Package and repository directory ownership.
* Largest Git packfiles.
* PostgreSQL database size.
* Referenced and unreferenced package blobs.
* Package ownership, largest packages, version ages, and cleanup rules.

Every section has a 120-second client timeout. A failed or timed-out section
does not suppress independent evidence from later sections. The command returns
nonzero when any section is incomplete and says which section failed on
standard error.

This is measurement only. It does not delete packages, run Git garbage
collection, truncate logs, recycle runners, or mutate Kubernetes resources.

## Layer boundary

Node-stats remains the general host and Kubernetes storage observer. It owns
root bytes and inodes, configured pressure paths, PVC attribution, and node
conditions without learning Forgejo's application schema. AOSguard owns this
application-specific permission surface and packaged bridge.

Infrastructure's attended measurement wrapper remains the rollout fallback
until the guarded command is present on the operator hosts. After rollout, that
wrapper can delegate to this command so the measurement procedure has one
owner.

## Mirror to GitHub

Forgejo is canonical and GitHub is a read-only downstream copy. Forgejo owns
release and tag creation, and GitHub does not become a second release source
unless a concrete consumer needs one. GitHub Releases are optional, and a repo
that needs one derives it from the Forgejo tag so Forgejo stays the single
source of truth.

`.forgejo/workflows/mirror-to-github.yml` keeps `coilysiren/agentic-os` in step
with canonical Forgejo `main`, no-ops without the `GITHUB_MIRROR_PAT` secret,
and runs behind a same-workflow test and pre-commit gate so the push only
happens when the repo's own checks passed. GitHub consumers import the mirrored
action library from `coilysiren/agentic-os/actions/*@main`, while Forgejo
consumers use fully qualified canonical URLs and do not depend on mirror
freshness.

## Fast-forward-only, never `--force`

GitHub `main` carries a cannot-force-push rule, and that rule **is** the PR gate
making GitHub the PR-gated downstream. So `git push github main` runs without
`--force` and fails if it is not a fast-forward, and `git push --tags github`
is append-only. Forgejo `main` is itself append-only, so in steady state every
push is a descendant of the GitHub tip and fast-forwards cleanly.

The old job ran `git push --force github main`, which GitHub rejected outright
(`GH013`). Because nobody watches mirror CI, the mirror sat two weeks stale
while Forgejo advanced 237 commits (agentic-os#309). The same failure hit
`session-lattice`, and mirror repos without the force-push rule synced fine.

## One-time reconcile

A rejected fast-forward means GitHub `main` carries commits that are not
ancestors of Forgejo `main`, usually GitHub-only hotfixes relanded on Forgejo
under different SHAs. The mirror cannot heal that without a `--force` the
protection rule forbids, so the job fails red rather than forcing, and a GitHub
admin repairs it once:

1. Confirm the divergent commits are already relanded on Forgejo by content, so
   the reset loses nothing.
2. Temporarily lift the rule or use an admin bypass, then
   `git push --force github <forgejo-main-sha>:main`.
3. Re-enable the rule.

Every subsequent push then fast-forwards cleanly. This is a one-time repair
rather than a routine release step, and no separate workflow template is needed:
the reusable part is the contract, implemented per repo by `promote.yml`,
`release.yml`, and `mirror-to-github.yml`.
