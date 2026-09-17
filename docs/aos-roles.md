# AOS roles

What a generic warded role selects.

## Generic warded roles

AOS no longer limits warded composition to Ward's fixed repository workflows.
Any safe lowercase role slug can use the generic runner:

```bash
aosward --agent codex --role story-architect --agent-id architect -- \
  "shape the premise and ask a critic to pressure-test it"
```

AOS translates this to:

```bash
ward agent run --role story-architect --agent-id architect \
  "shape the premise and ask a critic to pressure-test it"
```

The selected harness, image, model environment, and immutable context bundle
follow the same AOS-owned translation for every role.

If a matching Ward director broker is already running for that repository and
harness, the generic run joins its peer-message group automatically.

The distinction is authority, not identity:

* Every safe role uses Ward's read-only one-shot lifecycle, `director`, `science`,
  and `platform` included.
* A role slug selects composed context only. It cannot grant credentials,
  mounts, network access, or landing authority.

Within a Ward broker group, generic agents may launch other generic peers and
use Ward's authenticated message channel. Their derived peer capability cannot
select platform or science or invoke privileged broker operations.
