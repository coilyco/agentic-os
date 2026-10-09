# AWS SSM Parameter Inventory (agentic-os)

Focused pointer to the SSM parameters `agentic-os` code reads at runtime. The
canonical fleet-wide inventory (with rotation/runbook detail) is the generated
`agentic-os-kai/SSM.md`. This file records only the params this repo's tooling
consumes, next to the code that consumes them. All values are SecureString.
Resolve at runtime via `aosguard ops aws ssm get-parameter`, never paste an opaque
id or DSN into a tracked file.

## `/coilysiren/`

- `/coilysiren/gpg-secret-key` - shared armored GPG secret key imported on demand by `scripts/gpg-ssm` when the configured signing key is not yet local.
- `/coilysiren/gpg-passphrase` - shared GPG signing passphrase fetched on demand by `scripts/gpg-ssm` at sign time.

- `/coilysiren/aterm/vapid-key` - VAPID private key for aterm Web Push, made by `aterm vapid` and fed to the daemon as `ATERM_VAPID_KEY`. See `.agents/skills/tooling-aterm-client/references/web-push.md`. Created when push is turned on.

- `/coilysiren/aterm/write/android-keystore-b64` - base64 PKCS12 release keystore for the aterm Android APK, SecureString. Synced to the `ATERM_ANDROID_KEYSTORE_B64` Actions secret by `just sync-actions-secrets`. Losing it means the next build cannot update the installed app in place.
- `/coilysiren/aterm/write/android-keystore-password` - store and key password for that keystore, SecureString, synced to `ATERM_ANDROID_KEYSTORE_PASSWORD`.
- `/coilysiren/aterm/write/android-keystore-cert-sha256` - SHA-256 of that keystore's certificate, 64 hex characters with no colons, SecureString, synced to `ATERM_ANDROID_CERT_SHA256`. `verify` fails a build signed by any other key, instead of the phone refusing the update.

## `/forgejo/`

- `/forgejo/coilyco-ops/api-token` - Forgejo token used by `scripts/git-credential-forgejo-ssm.sh` for HTTPS git authentication.
