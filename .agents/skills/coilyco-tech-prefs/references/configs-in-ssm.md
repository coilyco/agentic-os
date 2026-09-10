# Configs go in SSM, not in skills or code

When a skill, script, or piece of code needs a config-shaped value (an account id, a voice id, a default channel, a board id, a URL, anything that might rotate or vary per host/account), stash it in AWS SSM and reference the parameter name. Do not hardcode the value into a SKILL.md body, a Python constant, a YAML default, or a checked-in JSON.

**Why:** single source of truth, rotatable without a code change, guarded access via `aosguard ops aws ssm`, swap-without-edit for environment changes, no stale duplicates across files. Hardcoded values rot silently the moment the upstream changes.

**How to apply:**
- Stash with `aosguard ops aws ssm put-parameter --name /<vendor>/<key> --type SecureString --value file:///ABSOLUTE/PATH/TO/VALUE_FILE`. Convention: vendor-scoped path, kebab-case leaf, SecureString even for non-secrets.
- Record the entry in `SSM.md` (canonical inventory at `~/projects/coilysiren/agentic-os-kai/SSM.md`) in the same commit.
- Reference it from code through the owning guarded SSM surface or, for shell
  sessions, fetch the exact parameter at point of use with
  `ssm-get /foo/bar-baz`. Do not load a parameter tree into the shell
  environment.
- In a SKILL.md, name the parameter and show the fetch command. Never paste the value into the body.

Applies to api keys (already obvious), but also to non-secret config: voice ids, channel ids, board ids, zone ids, agent ids, account-scoped identifiers. The bright line is "would I have to edit a file if this value changed". If yes, it goes in SSM.
