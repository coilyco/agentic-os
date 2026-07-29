---
doc_goal: Define the provider-neutral credential handoff from AOS to Ward.
---
# AOS to Ward credential handoff

AOS owns the coilyco deployment source for Ward's Forgejo broker credential.
Ward owns the broker, its authorization policy, lifecycle, and agent isolation.
Ward never learns how the deployment obtains the credential.

## Launch behavior

For an integrated `aos --warded` launch:

1. AOS uses a non-empty host `FORGEJO_TOKEN` as an explicit override.
2. Otherwise, AOS reads the deployment parameter through the host AWS session.
3. AOS supplies the value only in Ward's privileged process environment.
4. Ward keeps the raw value in its sibling broker.
5. Ward removes the value from the selected agent harness environment.

The credential stays in process memory. It never enters argv, dry-run output,
the context bundle, tracked configuration, or agent-visible files.

The direct host AWS read is a credential-bootstrap exception. AOS must obtain
the credential before Ward can expose any guarded operator surface. AWS and SSM
remain deployment details in AOS rather than dependencies in Ward.

An absent or unreadable deployment credential fails the launch before Ward
starts. The operator refreshes the host AWS session or supplies the explicit
environment override. AOS never asks the operator to paste the token.

## Boundaries

* Ward remains provider-neutral and accepts the credential as a launch input.
* AOSguard keeps its independent specgen-owned credential mounts.
* Standalone AOS launches do not gain this Ward broker credential.
* Agent-compose context bundles carry no credential or Ward authority.

## See also

* [AOS launch CLI](aos-cli.md) - integrated capability flags and routing.
* [Shell and secret handling](features-shell-secrets.md) - on-demand SSM reads.
* [AOS and Ward boundary](ward-specs.md) - deployment configuration ownership.
