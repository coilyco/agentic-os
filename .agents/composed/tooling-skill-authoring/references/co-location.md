# Per-repo co-location requirements

A repo may host its own `.agents/skills/` for pure design or usage reference skills that fire only when Claude Code operates in that repo's directory. When co-locating, the host repo must:

1. Receive the skill-discipline pre-commit hooks (`validate-skills`, `dead-cross-links`) plus the rest of the catalog suite via `make apply-agentic-os-hooks` from `agentic-os-kai`. That rollout inserts one managed `repo: https://github.com/coilysiren/agentic-os` block into the host's `.pre-commit-config.yaml`. No stamped local copies. The validators live in the `agentic_os` Python package. Pre-commit pip-installs them. See the skill-discipline rollout docs.
2. Ship a slim `.agents/skills/categories.yaml` at the skills root with only the categories the repo actually uses. The validator reads this path directly.
3. Run `pre-commit install` in the host repo. That activates the hooks for every commit.

No `setup.sh` is required in the host repo. Claude Code auto-discovers skills under any `.agents/skills/` in the working tree.
