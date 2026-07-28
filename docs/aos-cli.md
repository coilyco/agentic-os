---
doc_goal: Define the standalone AOS container launcher, its workspace and substrate contract, and the deferred release shape.
---
# AOS composed-container CLI

`aos` launches the dev-base image without Ward:

```bash
aos --role engineer acompose -- codex
```

The command after `--` is preserved exactly. Its executable selects the agent-compose layout unless `--layout` names one explicitly.
The [check-in adapter](aos-acompose-checkin.md) owns a bounded invocation while `--role` and `--delivery` become a normal request.
Claude and Codex compose the frontier catalog. The open-source Goose and OpenCode layouts compose the low-context catalog, keeping required skills and removing sources marked optional for smaller models.

The container accepts and ignores legacy `--density full` from an old launcher. Brief density is removed.

## Launch contract

Every launch supplies the same runtime base:

* The moving default image is pulled before each launch. Custom images keep Docker's local behavior.
* CWD mounts read-write at `/workspace/<cwd-name>` and becomes the workdir.
* `aos-substrate-cache` mounts at `/var/cache/aos/git`.
* The image hydrates fresh reference trees under `/substrate/<owner>/<repo>`.
* AOS copies the selected harness's read-only auth into the ephemeral HOME.
* Codex trusts the exact workspace, skips inner approvals, uses its full-access
  inner sandbox, defaults to Terra-medium, and hides rate-limit model nudges.
* Present API-key environment variables cross by name, never by rendered value.
* The Docker socket, AWS config, host HOME, and Git credentials do not mount.

Root performs bootstrap only. The harness runs as the host uid and gid with an ephemeral `/home/aos`, keeping a Linux bind-mounted workspace writable while the root-owned substrate stays read-only.

## Composition flow

The image carries self-contained `agent-compose`, its `acompose` alias, and the
AOS capability provider inside the baked substrate seed. Container bootstrap:

1. refreshes the shared mirrors and materializes reference checkouts
2. composes the requested role for the selected layout's model class
3. verifies the immutable bundle
4. projects it with `project --scope home` and execs the command after `--`

`--no-substrate` omits general reference trees but still materializes the AOS
capability provider required by the standard request. Personality context stays
inside agent-compose. The runtime invokes no Ward command, config, bootstrap,
authority, or lifecycle.

## Specialized role smoke

```bash
ward exec aos-build
ward exec aos-image-build
ward exec aos-designer-smoke
```

The smoke verifies designer instructions, skill promotion, no host auth or
unrelated substrate, and no engineer or QA composed sources.

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

* [dev-base-image.md](dev-base-image.md) - full image contents and publication.
* [aos-cli-release.md](aos-cli-release.md) - binary and package delivery.
* [personality-provider.md](personality-provider.md) - composed source.
* [FEATURES.md](FEATURES.md) - shipped inventory.
