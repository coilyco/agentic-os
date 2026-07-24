# Forgejo runner-token fetch overlay

`aguard ops forgejo actions generate-runner-token` mints a Forgejo Actions runner
registration token from the guarded surface. The HTTP request itself lives in
[`.ward/guardfile.forgejo.runnertoken.kdl`](../.ward/guardfile.forgejo.runnertoken.kdl)
as three declarative fetch leaves, one per scope:

- `global` - `/admin/runners/registration-token`
- `org <org>` - `/orgs/{org}/actions/runners/registration-token`
- `repo <owner> <repo>` - `/repos/{owner}/{repo}/actions/runners/registration-token`

The thin exec bridge in
[`.ward/guardfile.forgejo.runnertoken.exec.kdl`](../.ward/guardfile.forgejo.runnertoken.exec.kdl)
routes the scope form through
[`.ward/forgejo-runner-token.py`](../.ward/forgejo-runner-token.py) onto the
right fetch leaf. It does not reimplement auth or HTTP.

See also:

- [Role surface tiers](role-surface-tiers.md)
- [Ward spec bundle](ward-specs.md)
