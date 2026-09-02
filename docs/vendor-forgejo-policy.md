# Vendoring the Forgejo policy into deploy

How `coilyco-bridge/deploy` gets a copy of this repo's Forgejo operator policy,
and why it arrives as a push rather than a fetch.

## Why a push

Config lives at the lowest layer that fully determines it, is consumed only by
that layer or higher, and is never fetched downward. A shipped product does not
reach up into a reference repo for its own runtime config.

A CI-time fetch is the same coupling wearing a different hat: it makes deploy's
build depend on reaching this repo. So the bytes arrive by a push that deploy
then reviews and commits, which is the authoring-vs-rollout split already in
force everywhere else. Authored here, rolled out by a push.

## What moves

`.forgejo/workflows/vendor-forgejo-policy.yml` fires on a `release` push that
touches either file, and runs `scripts/ci/vendor-forgejo-policy.sh`:

- `.umbra/guardfiles/aosguard/forgejo.kdl` - the operator policy
- `.umbra/guardfiles/aosguard/forgejo.swagger.v1.json.gz` - the pruned spec

Both land in `services/forgejo-mcp/vendor/aosguard/` beside a `SOURCE` file
naming the commit they came from. The pin is what makes the copy auditable:
deploy answers "which policy is this" by reading it rather than guessing.

The script pushes a branch and opens a pull request. It never merges. Deploy
reviews and lands its own vendored copy, so nothing here writes deploy's `main`.

## The credential, and what happens without it

The push needs `DEPLOY_WRITE_TOKEN`, an Actions secret carrying write to
`coilyco-bridge/deploy`. Minting and placing it is an operator step on a hosted
surface, so it is not part of this change.

Until it exists the workflow is **inert rather than broken**: the script warns
and exits 0 on an absent token, the same guard `aos-cli-release.sh` uses for its
tap and scoop pushes. A run reports the skip in its log rather than failing the
release path.

That guard is the thing worth not losing. A vendoring job that fails loudly on
every release would get disabled; one that silently pushed with a token nobody
reviewed would be worse. `tests/test_vendor_forgejo_policy.py` holds it in place.

## Once it lands in deploy

`services/forgejo-mcp/forgejo.mcp.kdl` can `inherit` the vendored guardfile
instead of restating grants. That is the deduplication `agentic-os#1365` was
originally asking after, and it is available only for this rung: the sirens-echo
tier fixes every path to one repository and cannot inherit a parameterized one.
