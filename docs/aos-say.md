# `aos-say`

`aos-say` speaks short status messages from a shell or relay.

## Direct path

On macOS, `aos-say` runs `/usr/bin/say` directly.

```bash
ward exec aos-say -- --voice Samantha --rate 190 build done
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
ward exec aos-say -- build done
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
