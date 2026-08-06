---
doc_goal: Define AOS as the composition root for standalone and Ward-governed agent launches.
---
# AOS launch CLI

AOS exposes one launch shape with composed context and guarded tools always present.

The shared role slug selects context across enabled capabilities and never
transfers authority between tools. Standalone AOS applies bounded access gates such as [kubeconfig projection](aos-kubeconfig.md).

## Launch modes

Every AOS launch has two contexts:

* Agent-compose verifies and projects the selected role into a private staged home.
* AOS attaches standalone `aosguard`, which keeps its specgen credential mounts.

The compatibility flags `--composed` and `--guarded` remain accepted, including
explicit false values, but cannot disable either context.

`aos` and `aoscompose` name the standalone container `<role>-<suffix>`.
`aoscomposed` stays compatible. For `aoscompose`, first positional selects
role. A second harness overrides that role default. Auth is default-on. Use
`--auth=false` only for startup checks. Examples: `aoscompose engineer --version`
and `aoscompose engineer goose --version`.

`aosward` is the same executable with warded mode forced. It equals
`aos --warded`, but `--warded=false` cannot disable its Ward delegation:

```bash
aosward --agent codex --role engineer -- owner/repo#267
```

Warded mode uses Ward's fixed workflows and generic broker for Compose, lifecycle, and [credential handoff](aos-ward-credentials.md).

## Routing

In warded mode, arguments after `--` are Ward launch arguments. AOS passes the
selected image, agent, role, workspace request, and optional generic context
bundle to Ward. Harness model and effort settings stay in the
harness's own configuration surface. Standalone AOS derives only the Agent
Compose `model-tier` request from the selected runtime model or harness
default. Agent-compose carries behavioral identity and seat context. A Ward
workflow cannot change privileged surface. See [the context-bundle adapter](aos-context-bundle.md).

Ward ships the `director`, `qa`, and `engineer` repository workflows. Other safe
roles use its [generic read-only command](aos-generic-warded-roles.md). AOS rejects
incompatible agents and translated Ward flags before starting a container.

## Standalone contract

* The moving default image is pulled before each launch. Custom images keep Docker's local behavior.
* CWD mounts read-write at `/workspace/<cwd-name>` and becomes the workdir.
* Composition hydrates the baked provider through `aos-substrate-cache`.
* [Codex authentication](aos-codex-auth.md) fails closed before Docker, projects
  file-backed or direct macOS Keychain credentials read-only, and preserves
  `--auth=false` for startup checks.
* [MCP and tailnet projection](aos-standalone-connectivity.md) preserves the native standalone connectivity baseline.
* [Role-gated kubeconfig projection](aos-kubeconfig.md) mounts one explicit operator-selected source read-only for standalone director and ops launches.
* With `--auth=true`, present authentication variables cross by name, never rendered value.
* Docker socket, AWS config, host HOME, and Git credentials do not mount.

Root performs bootstrap only. The harness runs as the host uid and gid.
Composition verifies the immutable role bundle with `project --scope home`.
`--no-substrate` omits unrelated reference trees.

## Validation and release

`ward exec aos-test` runs Go. The `aos-composition-dry-run`,
`aos-composition-smoke`, and `aos-standalone-composition-smoke` Ward verbs
cover both lifecycle shapes. The standalone smoke uses `--auth=false` and a
version command, so it proves startup rather than authenticated inference. The
[Codex auth contract](aos-codex-auth.md) names the inference probe. The [CLI release
pipeline](aos-cli-release.md) publishes checksummed binaries and package metadata.

## See also

* [Context bundle](aos-context-bundle.md) - runtime adapter.
* [aosguard.md](aosguard.md) - guarded tool and generated skill.
