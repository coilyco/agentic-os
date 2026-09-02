# Ward specs and burndown policy

Ward owns fixed workflow commands, container isolation, lifecycle, repository
workflow mechanics, and its fixed broker. AOS does not ship a Ward role bundle.

The only Ward-consumed AOS file was [`.ward/ward.yaml`](../.ward/ward.yaml),
validated by `ward doctor` through Ward's loader. That runtime is out of AOS CI
under agentic-os#1299 and the file outlives it, so its surviving contract is
written out below rather than left in an archived repository.

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
  bounded [kubeconfig projection](aos-cluster-access.md). That standalone gate does
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

## The `.ward/ward.yaml` schema

Ward's own schema page goes read-only when that repository is archived, so the surviving contract lives here. Source: ward `docs/ward-yaml.md` at `040f159`, read before the archive. Fifteen of the sixteen repositories on this host declare the file, nothing in Ward reads it any more, and `catalog-trifecta` stopped requiring it fleet-wide (`coilysiren/inbox#385`).

* **`catalog.description` and `catalog.dependsOn`** - the cross-repo knowledge graph. Declared almost everywhere, and **no code on this host reads either one today**. An inventory a later consumer may pick up rather than a live input.
* **`capabilities`** - a list of `provider/skill-dir` strings, read by `agentic-os-kai/scripts/pull-capabilities.py` to pull capability skills down into one leaf repo. Never part of Ward's documented schema, and today only `coilyco-gaming/galaxy-gen` declares it.
* **Retiring with the runtime** - `agent.image`, `agent.workflow`, and `agent.release-channel` were Ward launch inputs, and the landing lane lives in AGENTS.md frontmatter instead. `commands` is retired because dev verbs are justfile recipes (`coilysiren/inbox#366`). `security` is retired because AOSguard and umbra own that surface.

**Two files, not one.** A separate `ward.yaml` at a repository root carries `tailnet.shortcut` and is fetched over the Forgejo API by `infrastructure/scripts/generate-caddy-shortcuts.py`, with `coily.yaml` and `config.yml` as migration fallbacks. Different path, different schema, different consumer. `pull-capabilities.py` accepts either path, which is the one place they meet.

**And the frontmatter key is a third thing.** `ward.workflow` in a repository's AGENTS.md selects one of four landing lanes and is read by `agentic_os.generators.generate_git_workflow`. Vocabulary rather than a runtime, so archiving Ward does not reach it.

## Release and validation

AOS releases do not attach a Ward-spec archive. The dev-base image carries the
released `ward`, `aos`, `aoscompose`, `aosward`, and `aosguard` binaries without a
checkout-derived Ward configuration reference.

**No AOS workflow installs `ward` any more.** Five steps across four workflows
pulled a release binary from a now-archived repository on every run, and three
of them never invoked it. The two that did ran `ward doctor` against a file the
same page records as read by nothing, so the gate validated a contract with no
consumer while adding a network dependency that could fail a job on its own.
The dev-base image still carries the binary, because a warded run needs it. The declared Go and Python
suites cover the always-composed, always-guarded standalone path and the Ward
context-bundle path.

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

## Frozen as a contract

Kai settled this on 2026-08-24 (agentic-os#1299). The schema, the `ward:` lane
vocabulary, and the shipped binary all stay. What came out is the runtime from
this repo's hot paths: the `ward-doctor` CI job, the workflow install steps, and
`ward --version` / `ward doctor` from the image's common verification.

Exposure rather than disuse: an unmaintained binary sat where a toolchain bump
breaking its install would read as a broken image build. The image still
installs Ward and provisions `~/.ward/audit`, so `ward agent` dispatch is
unchanged. Verification of a frozen component stopped, not the component.

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
