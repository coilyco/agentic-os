---
doc_goal: Define the reduced AOS to Ward integration boundary.
---
# AOS and Ward boundary

Ward owns fixed workflow commands, container isolation, lifecycle, repository
workflow mechanics, and its fixed broker. AOS does not ship a Ward role bundle.

The only Ward-consumed AOS file is [`.ward/ward.yaml`](../.ward/ward.yaml). It
declares repository development commands and the supported verification fixture
input. `ward doctor` validates that YAML through Ward's loader.

## Ownership

* Agent-compose owns behavioral roles, seats, identity, personalities, and
  composed skills in [`.agents/roles.kdl`](../.agents/roles.kdl).
* AOS owns the selected harness, image, deployment defaults, and the immutable
  context-bundle adapter.
* Ward receives the fixed workflow role, selected harness and image, original
  work request, broker credential, and optional context bundle. A role slug is
  composition metadata only. It grants no permissions.
* AOSguard owns the independent generated operator surface and its credential
  mounts. Ward neither imports nor configures it.

There are no AOS Ward role guardfiles, KDL defaults, role-seat projections,
broker grants, topology, network reach, or release-bundle assets. Image and
harness behavior reaches the selected harness through AOS and agent-compose,
not a Ward role overlay.

## Release and validation

AOS releases do not attach a Ward-spec archive. The dev-base image carries the
released `ward`, `aos`, and `aosguard` binaries without a checkout-derived
Ward configuration reference. The declared Go and Python suites include the
standalone composed, standalone guarded, and combined context-bundle paths.

## See also

* [AOS launch CLI](aos-cli.md)
* [AOS context-bundle adapter](aos-context-bundle.md)
* [aosguard](aosguard.md)
