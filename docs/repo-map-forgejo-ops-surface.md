# Forgejo ops surface discovery

Use this map when the question is "what Forgejo surface exists here now?"

If the same repo also exists under `/substrate`, work in `/workspace/agentic-os`.

## Find the current surfaces

- `aguard ops forgejo describe`
- `aguard ops forgejo --help`
- `rg -n "aguard ops forgejo" docs .specgen`

## First check

- `aguard ops forgejo describe`

## Notes

- Update [aguard](aguard.md) when the operator surface changes.
- Prefer runtime help before guessing a verb name.
