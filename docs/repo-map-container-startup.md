# Container startup and broker dispatch

Use this map when a run starts wrong, mounts the wrong root, or dispatch wiring looks stale.

If the same repo also exists under `/substrate`, work in `/workspace/agentic-os`.

## Find the current surfaces

- `rg -n "AOS_REPO_ROOT|ward agent|entrypoint" docker docs .ward`
- `ward exec dev-base-build`
- `ward exec warp doctor`

## First check

- `ward exec dev-base-build`

## Notes

- `ward agent` is the runtime entry point.
- `AOS_REPO_ROOT` is seeded by the entrypoint for the checked-out AOS source.
