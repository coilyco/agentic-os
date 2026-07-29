---
doc_goal: Define the reduced AOS to Ward integration boundary.
---
# AOS and Ward boundary

Ward owns fixed workflow commands, container isolation, lifecycle, repository
workflow mechanics, and its fixed broker. AOS does not ship a Ward role bundle.

The only Ward-consumed AOS file is [`.ward/ward.yaml`](../.ward/ward.yaml). It
declares repository development commands, the AOS image and release channel,
and the supported verification fixture input. `ward doctor` validates that
YAML through Ward's loader.

## Ownership

* Agent-compose owns behavioral roles, seats, identity, personalities, and
  composed skills in [`.agents/roles.kdl`](../.agents/roles.kdl).
* AOS owns the selected harness, concrete launch tuning in
  [`harness_launch_profiles.json`](../aos/harness_launch_profiles.json), image,
  deployment defaults, and the immutable context-bundle adapter.
* Ward receives the fixed workflow role, selected harness and image, original
  work request, explicit harness environment, broker credential, and optional
  context bundle. A role slug selects composition. Standalone AOS may also use
  it for launch tuning. It grants no permissions.
* AOSguard owns the independent generated operator surface and its credential
  mounts. Ward neither imports nor configures it.

There are no AOS Ward role guardfiles, KDL defaults, broker grants, topology,
network reach, or release-bundle assets. The former `.ward` concerns moved as
follows:

* Image, release channel, and this repository's landing workflow moved from
  `defaults.kdl` and `repos.kdl` to `.ward/ward.yaml`.
* Models, reasoning effort, verbosity, and local harness defaults moved from
  `agents.kdl` and `roles.kdl` to the embedded AOS launch-profile registry.
  Standalone AOS launches may select its role tuning. Ward-bound launches
  receive only the registry's harness-level defaults through Ward's explicit
  `WARD_*` environment seam, so a Ward workflow role cannot change those
  inputs.
* Role behavior and composed skills remain in `.agents/roles.kdl`.
* Seat names and pronouns remain canonical in agent-compose. AOS does not
  duplicate its person registry.
* Role-derived grants were retired rather than moved. Ward workflows and the
  separately selected AOSguard surface now determine executable authority.

## Release and validation

AOS releases do not attach a Ward-spec archive. The dev-base image carries the
released `ward`, `aos`, and `aosguard` binaries without a checkout-derived
Ward configuration reference. The declared Go and Python suites include the
standalone composed, standalone guarded, and combined context-bundle paths.

## See also

* [AOS launch CLI](aos-cli.md)
* [AOS context-bundle adapter](aos-context-bundle.md)
* [aosguard](aosguard.md)
