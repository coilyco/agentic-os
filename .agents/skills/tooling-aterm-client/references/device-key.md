# Device key unlock

The Android app types after proving it holds a key in the phone's Keystore, behind the fingerprint prompt. A page at https://tauri.localhost cannot assert the coilyco.dev passkey, so this stands in for it (COI-2623). Only Kai's own terminal grants it: the code comes from `aterm passkey enroll`, which refuses inside a session.

## What a person sees

* **No bridge or no daemon support** - "the app cannot unlock yet".
* **No screen lock** - one sentence, no controls, and no code sent.
* **A changed fingerprint** - the sentence, then the enrollment steps, since the old key no longer works.
* **Enrolling** - one tap on Set up key, then one fingerprint prompt. The daemon does not unlock on `device_enrolled`, so an assertion follows at once. A closed prompt keeps the key and says the Unlock button will work.
* **A key the host knows** - "Unlock with fingerprint". A phone redials on every wake, so each wake costs one tap and one prompt.
* **A key the host forgot** - the enrollment steps, with a sentence saying so.
* **A closed prompt** - one line, and the button stays.

## The plugin

Called as `window.__TAURI__.core.invoke("plugin:devicekey|status" | "enroll" | "sign")`. `status` never prompts, and runs before a code is sent. A rejection is `{code, message}` with a code of `cancelled`, `lockout`, `none_enrolled`, `unsupported`, `invalidated`, or `other`.

## The frames

`device_enroll_begin`, `_finish` and `device_assert_begin`, `_finish`, each answered by a challenge or `device_enrolled` and `device_asserted`. `typing` carries `device_key` standing beside `passkey`. The assertion names its `key_id`, and `device_key_unknown` means the host forgot it.

The daemon half, the frames and the attestation checks, is [device-key-daemon.md](device-key-daemon.md).
