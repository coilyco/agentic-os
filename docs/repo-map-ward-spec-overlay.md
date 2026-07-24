# Ward spec overlay and role-tier bundle

Use this map when the issue is about `.ward/`, role tiers, or the shipped spec overlay.

If the same repo also exists under `/substrate`, work in `/workspace/agentic-os`.

## Find the current surfaces

- `rg -n "WARD_CONFIG_REF|defaults.kdl|roles.*kdl|repos.kdl|ward-specs" .ward docs`
- `ward doctor`
- `aguard ops forgejo describe`

## First check

- `WARD_CONFIG_REF="file://${PWD}/.ward" ward doctor`

## Notes

- This repo authors the coilyco ward spec bundle.
- `defaults.kdl` owns the aos deployment image and moving tag.
- `roles.kdl` holds shipped per-harness agent overlays and generated,
  model-opaque AOSH intent routes inside the canonical role blocks.
