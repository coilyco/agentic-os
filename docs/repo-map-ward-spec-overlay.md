# Ward spec overlay and role-tier bundle

Use this map when the issue is about `.ward/`, role tiers, or the shipped spec overlay.

If the same repo also exists under `/substrate`, work in `/workspace/agentic-os`.

## Find the current surfaces

- `rg -n "WARD_CONFIG_REF|roles.*kdl|workflow.kdl|ward-specs" .ward docs`
- `ward doctor`
- `ward ops forgejo describe`

## First check

- `WARD_CONFIG_REF="file://${PWD}/.ward" ward doctor`

## Notes

- This repo authors the coilyco ward spec bundle.
- `roles.kdl` is where the shipped per-harness agent overlays live.
