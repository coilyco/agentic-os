---
doc_goal: Define AOS as the composition root for standalone and Ward-governed agent launches.
---
# AOS launch CLI

AOS exposes one launch shape with composed context and guarded tools always present.

The shared role slug selects context across capabilities and never transfers
authority. Standalone AOS applies [bounded access gates](aos-kubeconfig.md).

## Launch modes

Every AOS launch has two contexts:

* Agent-compose verifies and projects the selected role into a private staged home.
* AOS attaches standalone `aosguard`, which keeps its specgen credential mounts.

The compatibility flags `--composed` and `--guarded` remain accepted, including
explicit false values, but cannot disable either context.

`aos` and `aoscompose` name the standalone container `<role>-<suffix>`.
`aoscomposed` stays compatible. `aoscompose` uses Docker host networking.
First positional selects role, and a second harness overrides the default.
Auth is default-on. Use `--auth=false` only for startup checks:
`aoscompose engineer --version` or `aoscompose engineer goose --version`.

`aosward` is the same executable with warded mode forced. It equals
`aos --warded`, but `--warded=false` cannot disable its Ward delegation:

```bash
aosward --agent codex --role engineer -- owner/repo#267
```

Warded mode uses Ward's generic runner and broker for Compose, lifecycle, and [credential handoff](aos-ward-credentials.md).

## Routing

In warded mode, arguments after `--` go to Ward with the image, agent, role,
workspace request, and context bundle. Harness model and effort settings never
change composition. AOS gives Agent Compose the role, delivery mode, and the
role's first roster-supported compatibility tier. That tier is identical across
seats and unrelated to model or context-window size. Agent Compose owns identity
and seat context. Ward cannot change privileged surface. See the
[context-bundle adapter](aos-context-bundle.md).

Ward ships the `director`, `qa`, and `engineer` repository workflows. Other safe
roles use its [generic read-only command](aos-roles-and-voice.md). AOS rejects
incompatible agents and translated Ward flags before starting a container.

## Standalone contract

* Default image pulls each launch. Custom images stay local.
* Standalone uses native shadow: worktrees mount at `/workspace`, mapped CWD is workdir.
* HOME is copied to `/home/aos` from an allowlist.
* Composition hydrates the baked provider through `aos-substrate-cache`.
* [Codex authentication](aos-auth.md) fails closed before Docker, projects
  file-backed or direct macOS Keychain credentials read-only, and preserves
  `--auth=false` for startup checks.
* [Standalone connectivity](aos-standalone-connectivity.md) keeps host networking, MCP, and tailnet behavior.
* [Kubeconfig projection](aos-kubeconfig.md) mounts one operator-selected source read-only for any standalone role.
* With `--auth=true`, auth env names cross without rendered values.
* Host HOME, AWS, Git, and Docker stay out. Credentials use auth projection only.

Root performs bootstrap only. The harness runs as the host uid and gid.
Composition verifies the immutable role bundle with `project --scope home`.
`--no-substrate` omits unrelated reference trees.

## Validation and release

`ward exec aos-test` runs Go. The `aos-composition-dry-run`,
`aos-composition-smoke`, and `aos-standalone-composition-smoke` Ward verbs
cover both lifecycle shapes. The standalone smoke uses `--auth=false` and a
version command, so it proves startup rather than authenticated inference. The
[Codex auth contract](aos-auth.md) names the inference probe. The [CLI release
pipeline](aos-cli-release.md) publishes checksummed binaries and package metadata.

## See also

* [Context bundle](aos-context-bundle.md) - runtime adapter.
* [aosguard.md](aosguard.md) - guarded tool and generated skill.
