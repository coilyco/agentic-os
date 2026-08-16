# Repo maps

One page per cross-repo trace. Each map answers a question that spans
repositories, which is why it does not live in any one of them.

## Repo maps

Compact starting points for the high-churn warded workflow areas that keep sending agents down the wrong path.

If the same repo also exists under `/substrate`, `/workspace/agentic-os` is authoritative.

The maps are search-first, not frozen inventories. They point at the commands that rediscover the current files when the surface shifts.

## Maps

- [Ward PR workflow and director merge paths](repo-maps.md)
- [Forgejo ops surface discovery](repo-maps.md)
- [Container startup and broker dispatch](repo-maps.md)
- [Ward integration boundary](repo-maps.md)
- [Ward burndown containment and restoration evidence](ward-specs.md)

Each map stays small on purpose. Read the entry points first, then run the first check.

## Ward integration map

Use this map when a change crosses AOS, Ward, agent-compose, or AOSguard.

## Find the current surfaces

* `rg -n "context-bundle|--warded|--composed|--guarded" aos docs`
* `ward doctor`
* `aosguard ops forgejo describe`

## Notes

* `.agents/roles.kdl` owns behavioral composition.
* `.agents/harness-launch-profiles.yaml` owns role-to-default-agent mapping.
* `.ward/ward.yaml` owns repository commands, the deployment image and release
  channel, and supported YAML fixture input.
* Agent-compose owns named seats and pronouns.
* Ward owns fixed workflows and its broker. AOSguard owns its separate static
  operator surface.

## Container startup and broker dispatch

Use this map when a run starts wrong, mounts the wrong root, or dispatch wiring looks stale.

If the same repo also exists under `/substrate`, work in `/workspace/agentic-os`.

## Find the current surfaces

- `rg -n "AOS_REPO_ROOT|ward agent|entrypoint" docker docs .ward`
- `ward exec dev-base-build`
- `ward exec warp doctor`

## First check

- `ward exec dev-base-build`

## Notes

- `ward agent` is the runtime entry point.
- `AOS_REPO_ROOT` is seeded by the entrypoint for the checked-out AOS source.

## Forgejo ops surface discovery

Use this map when the question is "what Forgejo surface exists here now?"

If the same repo also exists under `/substrate`, work in `/workspace/agentic-os`.

## Find the current surfaces

- `aosguard ops forgejo describe`
- `aosguard ops forgejo --help`
- `rg -n "aosguard ops forgejo" docs .specgen`

## First check

- `aosguard ops forgejo describe`

## Notes

- Update [aosguard](aosguard.md) when the operator surface changes.
- Prefer runtime help before guessing a verb name.

## Ward PR workflow and director merge paths

Use this map when the issue is about PR lifecycle, director merge, or burn-down.

If the same repo also exists under `/substrate`, work in `/workspace/agentic-os`.

## Find the current surfaces

- `rg -n "pull-request-and-merge|director merge|WARD-OUTCOME|repos\\.workflow" .ward docs`
- `aosguard ops forgejo pr --help`
- `ward agent director --help`

## First check

- `aosguard ops forgejo issue view <owner> <repo> <issue>`

## Notes

- The issue thread carries the workflow and merge authorization.
- Use `aosguard ops forgejo pr view <owner> <repo> <pr>` after the PR exists.
