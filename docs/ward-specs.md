# Ward specs and burndown policy

## AOS and Ward boundary

---
doc_goal: Define the reduced AOS to Ward integration boundary.
---

Ward owns fixed workflow commands, container isolation, lifecycle, repository
workflow mechanics, and its fixed broker. AOS does not ship a Ward role bundle.

The only Ward-consumed AOS file is [`.ward/ward.yaml`](../.ward/ward.yaml). It
declares repository development commands, the AOS image and release channel,
and the repository landing workflow. `ward doctor` validates that YAML through
Ward's loader.

## Ownership

* Agent-compose owns behavioral roles, seats, identity, personalities, and
  composed skills in [`.agents/roles.kdl`](../.agents/roles.kdl).
* AOS owns the selected harness, role-to-default-agent mapping in
  [`harness-launch-profiles.yaml`](../.agents/harness-launch-profiles.yaml), image,
  deployment defaults, and the immutable context-bundle adapter.
* Ward receives the fixed workflow role, selected harness and image, original
  work request, broker credential, and optional context bundle. In the warded
  path, a role slug selects composition and grants no permissions. Standalone
  AOS may also use it for launch selection and its own
  bounded [kubeconfig projection](aos-kubeconfig.md). That standalone gate does
  not transfer a grant into Ward.
* AOSguard owns the independent generated operator surface and its credential
  mounts. Ward neither imports nor configures it.

There are no AOS Ward role guardfiles, KDL defaults, broker grants, topology,
network reach, or release-bundle assets. The former `.ward` concerns moved as
follows:

* Image, release channel, and this repository's landing workflow moved from
  `defaults.kdl` and `repos.kdl` to `.ward/ward.yaml`.
* Role default-agent selection moved from `agents.kdl` and `roles.kdl` to the
  embedded AOS launch-profile YAML registry. Harness model, effort, verbosity,
  endpoint, and local defaults remain harness-owned. Ward-bound launches do not
  receive AOS-owned `WARD_*` model environment.
* Role behavior and composed skills remain in `.agents/roles.kdl`.
* Seat names and pronouns remain canonical in agent-compose. AOS does not
  duplicate its person registry.
* Role-derived command grants were retired rather than moved. Ward workflows
  and the separately selected AOSguard surface determine executable authority.
  AOS owns only its bounded standalone runtime inputs, including kubeconfig
  projection.

## Release and validation

AOS releases do not attach a Ward-spec archive. The dev-base image carries the
released `ward`, `aos`, `aoscompose`, `aosward`, and `aosguard` binaries without a
checkout-derived Ward configuration reference. The declared Go and Python
suites cover the always-composed, always-guarded standalone path and the Ward
context-bundle path.

## See also

* [AOS launch CLI](aos-cli.md)
* [AOS context-bundle adapter](aos-context-bundle.md)
* [aosguard](aosguard.md)

## Ward burndown policy

Ward's issue burndown stays deny-by-default until actor-aware admission lands.
Autonomous triage can only run where a repo is explicitly opted in.

## Current containment

- `burndown default=#false` is the fleet default.
- Opt-in is per repo and explicit. A small number of repos carry it, all of them unreachable for writes from the public Forgejo API.
- Public repos stay available for normal issue reporting and human triage, but they do not inherit autonomous execution.

The roster itself lives in Ward's actor-aware queue filter and the host-local
`director.default-scope`, which are the surfaces that decide admission. This doc
records the policy, not the list.

## Rollout order

1. Keep deny-by-default containment in Ward's actor-aware queue filter, with
   the candidate set selected through host-local `director.default-scope`.
2. Add a repo-level external issue-admission setting in Ward.
3. Require the setting to name the approval mode and configured automation actors.
4. Fail closed when the setting is missing or unknown.
5. Re-enable an external tracker only after Ward provenance checks and snapshot tests pass.

## Admission rule

- Externally writable from the public Forgejo API - denied, no exceptions, whoever owns the repo.
- Not externally writable - eligible for an explicit opt-in, and denied until it has one.
- Unreachable from the public API is necessary but not sufficient. Repos in that state are still denied by default, because containment tracks the trust of the issue actor rather than the reachability of the repo.
- Neither state is inherited. A repo that changes writability keeps its current admission until the opt-in is revisited deliberately.

## Acceptance trail

- The fleet docs distinguish repository-owner trust from issue-actor trust.
- Every return to autonomous burndown needs the Ward issue provenance checks and snapshot tests to pass.
- The parent program links this inventory and the staged restoration evidence.
