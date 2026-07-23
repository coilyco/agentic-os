---
doc_goal: Define the standalone AOS container launcher, its workspace and substrate contract, and the deferred release shape.
---
# AOS composed-container CLI

`aos` launches the dev-base image without Ward:

```bash
aos --role engineer acompose -- codex
```

The command after `--` is preserved exactly. Its executable selects the
agent-compose layout unless `--layout` names one explicitly. `--role`,
`--density`, and `--delivery` become a normal agent-compose request.

## Launch contract

Every launch supplies the same runtime base:

* CWD mounts read-write at `/workspace/<cwd-name>` and becomes the workdir.
* `aos-substrate-cache` mounts at `/var/cache/aos/git`.
* The image manifest hydrates fresh reference trees under
  `/substrate/<owner>/<repo>`.
* The selected harness's known auth file mounts read-only when present. AOS
  copies it into the ephemeral HOME before dropping privileges.
* Present API-key environment variables cross by name, never by rendered value.
* The Docker socket, AWS config, host HOME, and Git credentials do not mount.

Advisor and director additionally use AOS's [private live-observe
composition](aos-private-network.md). All other roles retain the runtime base
above without a private network or proxy.

Root performs bootstrap only. The harness runs as the host uid and gid with an
ephemeral `/home/aos`, keeping a Linux bind-mounted workspace writable while
the root-owned substrate stays read-only.

## Composition flow

The image carries `agent-compose`, its `acompose` alias, and the AOS provider
snapshot inside the baked AOS substrate seed. Container bootstrap:

1. refreshes the shared mirrors from the image seeds without network
2. materializes the reference checkouts
3. composes the requested role
4. verifies the immutable bundle
5. projects it with `project --scope home`
6. execs the command after `--`

`--no-substrate` omits general reference trees but still materializes the AOS
provider required for composition. The runtime invokes no Ward command,
config, bootstrap, authority, or lifecycle.

## Ownership

AOS owns the reusable runtime substrate: image seeds, public roster, CWD
mount, cache hydration, workspace and reference topology, and composed HOME.
Ward owns governed execution layered above it: fresh target clones, writable
repo grants, credential scopes, issues, reservations, reaping, and landing.
Ward can later consume this primitive without moving those policies into AOS.

## Release

The [CLI release pipeline](aos-cli-release.md) validates every main push and
publishes the next minor `aos-v*` release. It cross-compiles checksummed
binaries, renders Homebrew and Scoop metadata, and stamps `aos version`.
Manual dispatch retries a tag or selects patch, minor, or major.

## See also

* [dev-base-image.md](dev-base-image.md) - image tiers and publication.
* [aos-cli-release.md](aos-cli-release.md) - binary and package delivery.
* [personality-provider.md](personality-provider.md) - composed source.
* [FEATURES.md](FEATURES.md) - shipped inventory.
