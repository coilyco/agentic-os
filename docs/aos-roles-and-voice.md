# AOS roles and voice

What a generic warded role selects, and how a session speaks.

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

* Every safe role uses Ward's read-only one-shot lifecycle, `tpm`, `eval`,
  and `platform` included.
* A role slug selects composed context only. It cannot grant credentials,
  mounts, network access, or landing authority.

Within a Ward broker group, generic agents may launch other generic peers and
use Ward's authenticated message channel. Their derived peer capability cannot
select platform or eval or invoke privileged broker operations.

## `aos-say`

`aos-say` speaks short status messages from a shell or relay.

## Direct path

On macOS, `aos-say` runs `/usr/bin/say` directly.

```bash
just aos-say --voice Samantha --rate 190 build done
```

Flags:

- `--voice` - pass a voice name to `/usr/bin/say`.
- `--rate` - pass a speech rate to `/usr/bin/say`.
- `--dry-run` - print the command or relay request without speaking.
- `--notification` - also post a desktop notification after speech.

## Relay path

On Linux or in a ward container, `aos-say` sends one JSON request to the configured relay.

```bash
export AOS_SAY_RELAY=unix:/tmp/aos-say.sock
just aos-say build done
```

The relay entrypoint is `aos-say relay`. It reads one request from stdin, runs `/usr/bin/say` by argv, and exits. That keeps the launchd side compatible with socket activation setups that hand the accepted connection to stdin.

## Request shape

The wire request is JSON:

```json
{"text":"build done","voice":"Samantha","rate":190,"dry_run":false,"notification":false}
```

## Notes

- No shell eval is used anywhere.
- The text payload stays one argv element all the way to `/usr/bin/say`.
