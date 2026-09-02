# aosguard

`aosguard` is AOS's guarded operator CLI. umbra remains generic,
while AOS owns this concrete policy snapshot, release name, and integration.

Packaged `umbra` discovers the [guardfile project](../../../../.umbra/README.md),
materializes generated Go out of band, and emits `aosguard` without committed
Go build glue. Dev-base and native AOS releases build from the same source and
lock, and Homebrew and Scoop install it beside `aos`.

## Authority boundary

The snapshot carries Forgejo, AWS, kubectl, helm, Tailscale, Actions, Netlify, and runner-token leaves, and excludes Ward role policy. AWS SSM permits single reads, file-backed writes, and named deletions. Actions lives at `aosguard ops actions` so its exec transport does not shadow `aosguard ops forgejo`, and carries only the log bridge. The sibling [`forgejo-storage measure` bridge](forgejo-ops.md) uses fixed `kubectl exec` operations from an embedded script invoked by absolute path.

`aosguard ops helm` is the second half of the cluster-write surface, and requires `--kube-context` on every call the way `ops kubectl` requires `--context`. It goes one step further and refuses `--kubeconfig` on every leaf, because an allowlisted context **name** is only a pinned cluster when the file resolving that name is fixed too. Release destruction mounts as `uninstall`, marked irreversible and cluster-pinned, so attended deprovision is audited instead of reaching bare helm; the `delete` alias stays denied. The matching kubectl gap is agentic-os#1506.

## Netlify domain aliases

`aosguard ops netlify` is the domain-alias surface for the one site this estate owns. Its token resolves from SSM at exec time the way `aosguard ops actions` resolves the Forgejo one, so it never sits in a caller's environment. Two leaves mount, `site` for the read and `alias` for the write, and both run the same packaged module, so nothing else in the Netlify API is reachable.

**Every write sends the whole alias list, and every change goes in one call.** The API replaces `domain_aliases` rather than merging into it, so a call carrying one alias would delete the rest. The module reads the current list, applies `--alias` and `--remove` to it, and sends the result, which is correct whichever way the API behaves and stops the caller having to know. A rename is therefore one write rather than two. It refuses a name equal to the site's primary domain, a `--remove` the site does not carry, a name both added and removed, and a call asking for neither. Batching is a safety property rather than tidiness: adding a domain re-issues the certificate covering every name on the site, the primary domain included, so three calls are three certificate events on a live site. This is the only step in adding a vanity domain that can affect the primary name.

**`--site` is required and allowlisted**, because a wrap that documents a fixed target while accepting any is the defect agentic-os#1349 closed on the kubectl surface, and a new surface is where that class returns. The site is named in full, since the API's other accepted form is an opaque uuid that has no business in a tracked file.

Forgejo pin actions are fixed to a single tracker where coilyco-ops holds
admin. Both wrappers read their credential from SSM through the same `provider
ssm` shape, so it never enters argv, logs, or tracked files.

Ward's fixed broker and AOSguard's static operator surface are independent, and
neither imports role-derived grants from the other.

## Generated skill

Specgen renders one concise native skill, `aosguard/SKILL.md`, plus a complete
lazy command index at `aosguard/references/commands.yaml`.

The full image builds those beside the binary and `aos --guarded` projects the
skill into the selected agent's skill root. In warded mode AOS also puts the
binary under the generic bundle's `bin/`, which Ward mounts read-only after the
image's PATH so it cannot shadow an image tool. The skill grants no permission:
`aosguard --help`, nested group help, and `describe` stay authoritative.

## Source ownership

`.umbra/guardfiles/aosguard/` owns the operator policy, vendored Swagger
inputs, and generated API locks. Ward owns its broker internally, neither
product reads policy from the other, and no drift check forces them to match.

The Forgejo source is vendored from the pruned deployment contract, so
`aosguard-lock` refreshes the dependency graph without reaching a live Forgejo
edge. Swagger and its lock use deterministic gzip and umbra decodes each
before use, and the resulting `specverb.lock` pins umbra for reproducible
builds. `aosguard-lock` refreshes the native skills under ignored
`dist/skills/`, while maintained documentation stays under `docs/`.

## Development

`just aosguard-build` materializes `dist/aosguard` and refreshes the generated
skills: umbra writes the `aosguard` index, then `generate_aosguard_skills`
splits it into one `aosguard-<area>` skill per wrapped entity. Hand-written
`tooling-aosguard` carries what no spec can (agentic-os#1028). `just aosguard-run --` passes subsequent arguments to the
generated command. `just aosguard-lock` is the only lock-writing step and
uses the packaged `umbra` executable.

Cross-repository composition is tracked on the intake tracker, with AOS
implementation in [agentic-os#755](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/issues/755).

## The Forgejo admin wrapper

`forgejo-admin` holds repo settings and cosmetics, org labels, repo topics, and branch protection. Forgejo refuses every one of them to `coilyco-ops`, an org member holding push rather than an owner holding repo-admin, so the credential is already a sufficient gate. Narrowing each action to its safest field on top of that cost more than it bought, so `repo edit` exposes the whole `EditRepoOption`, the same field set `infrastructure/scripts/forgejo-repo-settings.py` converges.

`private` and `archived` ride along rather than being blocked, because anyone
who can reach this wrapper can already flip either in the UI in fewer steps.

Deletes stay marked: `org-label delete` strips the label from every issue
carrying it, so the guardfile names `edit org-label` as the rename path, which
keeps the label id and every association. Org-label writes on the ordinary
wrapper are `never` leaves naming the admin verb, rather than the bare
`403 Must be an organization owner` they used to answer.

`create label` is here rather than on the ordinary wrapper because Forgejo mints labels per organization and `coilysiren` is a user account, so its six repos have no org to hang one on. The ward#107 deny stays exactly where it was: a repo label duplicating an org label silently shadows it, so on an org-owned repo `create org-label` is still the verb.

This wrapper carries **no vendored `.swagger.v1.json.gz`**, unlike `forgejo.kdl`. A vendored snapshot is pruned to the operations declared when it was written, so it cannot grow a new one: `issueCreateLabel` is in live Forgejo and was absent from the committed 88-operation copy. Dropping it means `umbra lock` fetches live for this guardfile, which costs hermetic locking and is why the other guardfile keeps its snapshot. Kai's call, 2026-08-29 (#1377).

## What it covers

Repo settings and cosmetics, org labels, topics, branch protection, Actions
secrets for repos and orgs, runner registration tokens, package retention,
repository contents, and the bot's personal access tokens.

The surface grew because provisioning scripts were reading the PAT into a shell
variable and passing it to `curl`. A guarded verb keeps the credential inside
the binary, so it never reaches a variable, a log, or `ps`. That is the point of
the wrapper, and a script that fetches the token itself defeats it however
carefully it handles it afterwards.

Two entries are credential-shaped rather than config-shaped and are worth
reading twice. `user-token create` returns the only copy of a new PAT, so its
stdout is the secret. `package delete` and `user-token delete` are irreversible.

## What it does not do, deliberately

It does not create or delete Forgejo users, and it does not create
organizations. Those are `never` leaves carrying the reason, so the boundary is
executable rather than a convention someone has to remember.

The line is that aosguard operates the estate and does not create principals in
it. A guard that can mint a user can mint one with any rights, which makes every
other restriction on the wrapper decorative. Kai's call, 2026-08-16.

The two bootstrap scripts that need those operations -
`provision-coilyco-ops-bot.sh` and `grant-coilyco-ops-org-repo-create.sh` in
`coilyco-bridge/infrastructure` - keep reading `/forgejo/admin-token` from
SSM directly. They run approximately once, so the standing capability a verb
would create costs more than the direct read.

## Credentials

The PAT comes from `/forgejo/admin-token` through the same `provider ssm` block
the ordinary wrapper uses, resolved at call time. There is no environment
variable to export and no wrapper script to run first. Access to the parameter
is the boundary, where an SSM-backed credential's boundary belongs.
