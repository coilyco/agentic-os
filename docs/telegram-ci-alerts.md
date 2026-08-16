# Telegram CI failure alerts

`aosguard ops telegram alert` posts a CI or CD failure to the in-cluster
`signoz-telegram` mapper. It is the target shape for every repo: one verb, no
arguments, and no alert program checked into any repository.

## Shape

```yaml
- name: Alert Telegram on main failure
  if: ${{ failure() && github.ref == 'refs/heads/main' }}
  continue-on-error: true
  run: aosguard ops telegram alert
```

The message is exactly three lines:

```
coilyco-bridge/deploy CI failing
workflow: deploy-galaxy-gen
run: https://forgejo.coilysiren.me/coilyco-bridge/deploy/actions/runs/4821
```

## Contract

- The caller passes nothing. Every field is read from the runner's own
  `GITHUB_*` environment, and a missing one degrades to `?` rather than
  raising, because a partial alert beats no alert.
- `ALERT_KIND=CD` switches the first line for a deploy job. `REPO`, `WORKFLOW`,
  `RUN_URL`, `FORGE_URL`, and `ALERT_URL` override their defaults.
- The run link is built from the forge `ROOT_URL`. `GITHUB_SERVER_URL` is the
  cluster-local name the runner registered against, so a link built from it is
  unreachable from a phone.
- No caller holds a Telegram credential. The mapper resolves the API base URL
  and chat id from pod environment.
- The leaf is `sealed`, so it forwards its pinned command exactly and takes no
  trailing arguments. The program is embedded in the binary rather than read
  from disk.
- Alert delivery is non-blocking. Keep `continue-on-error: true` so a mapper
  outage cannot obscure the job that originally failed.

## Availability

The verb needs `aosguard` on the job. Every workflow that sets
`container: forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release` has
it. A job on the `deploy:host` executor does not, because host-executor steps
run in the runner pod rather than that image.

## Migration

Four implementations of this alert exist across the fleet and the verb replaces
all of them:

- `coilyco-flight-deck/agentic-os` - the local `actions/telegram-alert`
  composite action, still in use by this repo's own workflows.
- `coilyco-flight-deck/infrastructure` - `scripts/actions/alert-telegram.py`,
  the authored source the rollout copies.
- `coilyco-bridge/deploy` - `scripts/alert-telegram.py`.
- `coilyco-bridge/agentic-os-kai` - `scripts/ci/alert-telegram.py`.

Consumers migrate only after this verb ships in a released image, so the
sequencing is: land the verb, let the image republish, then move call sites and
delete each implementation. The rollout side lives in
`coilyco-flight-deck/infrastructure`.

## See also

- [aosguard.md](aosguard.md)
- [features-release-tooling.md](features-release-tooling.md)
- [README.md](../README.md)
