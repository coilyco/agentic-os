# Forgejo runner-token fetch overlay

`aosguard ops forgejo actions generate-runner-token` mints a Forgejo Actions
runner registration token from AOSguard's standalone generated surface. Its
three scoped leaves are:

- `global` - `/admin/runners/registration-token`
- `org <org>` - `/orgs/{org}/actions/runners/registration-token`
- `repo <owner> <repo>` - `/repos/{owner}/{repo}/actions/runners/registration-token`

The generated AOSguard command owns request routing, authentication, and HTTP.
Ward does not consume this surface or mount its credentials.

See also:

- [aosguard](aosguard.md)
- [AOS and Ward boundary](ward-specs.md)
