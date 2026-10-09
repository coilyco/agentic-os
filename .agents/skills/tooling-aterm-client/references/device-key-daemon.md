# The Android app's device key, daemon side

The app types with a key held in the phone's hardware, not a WebAuthn passkey (COI-2623). A page at `https://tauri.localhost` cannot assert for the `coilyco.dev` relying party, and WebView passkeys need a public Digital Asset Links file past the gate. The code is `aterm/daemon_devicekey.go` and `devicekey_attest.go`. The client half is [device-key.md](device-key.md).

**Only Kai's terminal grants typing.** `aterm passkey enroll` mints the one-time code, and `aterm passkey revoke` forgets every passkey and device key. Both refuse inside a session. A code enrolls either credential, and a device key is stored beside the passkeys in `passkeys.json`.

**Frames**, on a web connection, with every binary field base64url and no padding:

* `device_enroll_begin {enroll_code}` answers `device_enroll_challenge {challenge}`: 32 random bytes, 60 s, single use, bound to the connection, spent by any finish.
* `device_enroll_finish {public_key, attestation}` answers `device_enrolled {key_id}`. `public_key` is SPKI DER of a P-256 key. `attestation` is an array of DER certificates, leaf first (one comma-joined string is also read). `key_id` is base64url SHA-256 of the SPKI. Enrolling does not unlock typing.
* `device_assert_begin {key_id}` answers `device_assert_challenge {challenge}`. A key not enrolled here is the error reason `device_key_unknown`.
* `device_assert_finish {key_id, signature}` answers `device_asserted`, then a `typing` push. The signature is ECDSA P-256 SHA-256 in DER over `"aterm-device-assert-v1" || 0x00 || challenge`. Every new connection asserts again. There is no grace window.
* `welcome` lists `device-key`, and `typing.device_key` is `enrolled` or `unenrolled`.

**Enrollment verifies the attestation instead of trusting it.** The chain must lead to one of the two roots Google publishes at android.googleapis.com/attestation/root, pinned in `devicekey_roots.go`. The leaf's attestation challenge must equal the issued one, its public key must equal `public_key`, both security levels must be TrustedEnvironment or StrongBox, and the hardware list must say EC, P-256, sign, a nonzero `userAuthType`, with `noAuthRequired` absent from both lists. Software-only attestation is refused. Google's attestation revocation list is not checked.

**The app's origin `https://tauri.localhost` is always allowed.** `ATERM_DAEMON_ALLOW_ORIGINS` replaces the default list rather than adding to it, so the origin is in code. Admission to the tailnet listener is unchanged.
