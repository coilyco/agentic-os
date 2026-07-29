---
doc_goal: Define AOS as the composition root for standalone and Ward-governed agent launches.
---
# AOS launch CLI

AOS exposes one launch shape for three independently useful capabilities:

```bash
aos \
  --agent codex \
  --role engineer \
  --warded \
  --composed \
  --guarded \
  -- owner/repo#267
```

The shared role slug selects context only. A matching name never transfers
permissions or unions authority between tools.

## Capability flags

* `--warded` - AOS invokes Ward for Compose, lifecycle, and
  [credential handoff](aos-ward-credentials.md).
* `--composed` - agent-compose verifies and projects the selected role into a
  private staged home.
* `--guarded` - AOS attaches standalone `aosguard`, which keeps its specgen credential mounts.

The flags stay independent. `--warded` uses Ward's fixed workflow and broker
surface. Either context capability can join it. All three flags enable the full
launch.

Without `--warded`, AOS owns one standalone container. Arguments after `--`
become arguments to the selected agent:

```bash
aos --agent codex --role engineer --composed -- --version
aos --agent codex --role engineer --guarded
```

## Routing

In warded mode, arguments after `--` are Ward launch arguments. AOS passes the
selected image, agent, role, workspace request, optional generic context
bundle, and harness-level model inputs through Ward's explicit environment
seam. Agent-compose carries behavioral identity and seat context. A Ward
workflow role cannot change the model inputs or privileged surface. See [the context-bundle
adapter](aos-context-bundle.md).

Ward ships `director`, `qa`, and `engineer`. Other agent-compose roles remain
available on the standalone composed path. AOS rejects incompatible roles,
agents, and translated Ward flags before starting a container.

## Standalone contract

* The moving default image is pulled before each launch. Custom images keep Docker's local behavior.
* CWD mounts read-write at `/workspace/<cwd-name>` and becomes the workdir.
* Composition hydrates the baked provider through `aos-substrate-cache`.
* AOS copies the selected harness's known read-only auth into ephemeral HOME.
* [MCP and tailnet projection](aos-standalone-connectivity.md) preserves the native standalone connectivity baseline.
* Present API-key environment variables cross by name, never rendered value.
* Docker socket, AWS config, host HOME, and Git credentials do not mount.

Root performs bootstrap only. The harness runs as the host uid and gid.
Composition verifies the immutable role bundle with `project --scope home`.
`--no-substrate` omits unrelated reference trees.

## Validation and release

`ward exec aos-test` runs Go. The `aos-composition-dry-run`,
`aos-composition-smoke`, and `aos-standalone-composition-smoke` Ward verbs
cover both lifecycle shapes. The [CLI release pipeline](aos-cli-release.md)
publishes checksummed binaries plus Homebrew and Scoop metadata.

## See also

* [Context bundle](aos-context-bundle.md) and [local lane profiles](local-lane-profiles.md) - runtime adapters.
* [aosguard.md](aosguard.md) - guarded tool and generated skill.
