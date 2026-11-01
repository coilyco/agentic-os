# aosguard

`aosguard` is AOS's guarded operator CLI. umbra remains generic,
while AOS owns this concrete policy snapshot, release name, and integration.

Packaged `umbra` discovers the [guardfile project](../../../../.umbra/README.md),
materializes generated Go out of band, and emits `aosguard` without committed
Go build glue. Dev-base and native AOS releases build from the same source and
lock, and Homebrew and Scoop install it beside `aos`.

## Authority boundary

The snapshot carries AWS, kubectl, helm, Tailscale and Netlify leaves, plus Forgejo, Actions and runner-token leaves that retired with Forgejo on 2026-11-01 and are cleanup under COI-2931. It excludes Ward role policy. AWS SSM permits single reads, SecureString-only writes, and named deletions. Three denies that guarded this surface are **deliberately absent** since 2026-09-02, Kai's call rather than collateral from the same edit: a `*/prod/*` and `*secret*` deny on `get-parameter`, a `*tfstate*` and `*-backup*` deny on `s3 ls`, and the `file:///*` value rule on `put-parameter`. The last one is worth naming twice, because SSM doctrine still says to pass `--value file:///...` so a secret never enters argv: that is now convention rather than enforcement, live since aos-v0.295.0. `put-parameter` gained `when type matches SecureString`, which fails closed on an absent flag and is the surface's one mandatory-value rule. The retired Forgejo leaves are described in [forgejo-ops.md](forgejo-ops.md) and [forgejo-actions-runs.md](forgejo-actions-runs.md).

`aosguard ops helm` is the second half of the cluster-write surface, and requires `--kube-context` on every call the way `ops kubectl` requires `--context`. It goes one step further and refuses `--kubeconfig` on every leaf, because an allowlisted context **name** is only a pinned cluster when the file resolving that name is fixed too. Release destruction mounts as `uninstall`, marked irreversible and cluster-pinned, so attended deprovision is audited instead of reaching bare helm. The `delete` alias stays denied. The matching kubectl gap is agentic-os#1506.

## Netlify domain aliases

`aosguard ops netlify` is the domain-alias surface for the one site this estate owns. Its token resolves from SSM at exec time, so it never sits in a caller's environment. Two leaves mount, `site` for the read and `alias` for the write, and both run the same packaged module, so nothing else in the Netlify API is reachable.

**Every write sends the whole alias list, and every change goes in one call.** The API replaces `domain_aliases` rather than merging into it, so a call carrying one alias would delete the rest. The module reads the current list, applies `--alias` and `--remove` to it, and sends the result, which is correct whichever way the API behaves and stops the caller having to know. A rename is therefore one write rather than two. It refuses a name equal to the site's primary domain, a `--remove` the site does not carry, a name both added and removed, and a call asking for neither. Batching is a safety property rather than tidiness: adding a domain re-issues the certificate covering every name on the site, the primary domain included, so three calls are three certificate events on a live site. This is the only step in adding a vanity domain that can affect the primary name.

**`--site` is required and allowlisted**, because a wrap that documents a fixed target while accepting any is the defect agentic-os#1349 closed on the kubectl surface, and a new surface is where that class returns. The site is named in full, since the API's other accepted form is an opaque uuid that has no business in a tracked file.

Wrappers read their credential from SSM through a `provider ssm` shape, so it
never enters argv, logs, or tracked files.

Ward's fixed broker and AOSguard's static operator surface are independent, and
neither imports role-derived grants from the other.

## Generated skill

Specgen renders one concise native skill, `aosguard/SKILL.md`, plus a complete
lazy command index at `aosguard/references/commands.yaml`.

The full image builds those beside the binary and `aos --guarded` projects the
skill into the selected agent's skill root. The skill grants no permission:
`aosguard --help`, nested group help, and `describe` stay authoritative.

## Source ownership

`.umbra/guardfiles/aosguard/` owns the operator policy, vendored Swagger
inputs, and generated API locks. Ward owns its broker internally, neither
product reads policy from the other, and no drift check forces them to match.

The retired Forgejo guardfiles and their vendored swagger snapshots stay
under that directory until COI-2931 removes them. Swagger and its lock use
deterministic gzip and umbra decodes each
before use, and the resulting `specverb.lock` pins umbra for reproducible
builds. `aosguard-lock` refreshes the native skills under ignored
`dist/skills/`, while maintained documentation stays under `docs/`.

## Development

`just aosguard-build` materializes `dist/aosguard` and refreshes the generated
skills: umbra writes the `aosguard` index, then `generate_aosguard_skills`
splits it into one `aosguard-<area>` skill per wrapped entity. Hand-written
`tooling-aosguard` carries what no spec can (agentic-os#1028). `just aosguard-run --` passes subsequent arguments to the
generated command. `just aosguard-lock` is the only lock-writing step and
uses the packaged `umbra` executable. `just guard-controls` runs `umbra controls` over both roots in the repo gate: every `never` and `withhold` must refuse by its own text, and removing it must change the outcome.

Cross-repository composition is tracked on the intake tracker, with AOS
implementation in agentic-os#755 (an archived Forgejo ref).

## The Forgejo admin wrapper (retired)

`forgejo-admin` held repo settings, org labels, topics, branch protection, Actions secrets, runner registration tokens, package retention, repository contents and the bot's personal access tokens. It retired with Forgejo on 2026-11-01, and its guardfile and `/forgejo/admin-token` credential path are cleanup under COI-2931.

Two design calls from it are kept as dated history. A guarded verb keeps a credential inside the binary, so a script that reads the token into a variable defeats the wrapper however carefully it handles it afterwards. And aosguard operates the estate and does not create principals in it, because a guard that can mint a user can mint one with any rights (Kai's call, 2026-08-16).
