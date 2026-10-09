# The Android device key

The aterm app types into a seat by proving it is Kai's phone. A passkey cannot do it, because the page at `https://tauri.localhost` is not the `coilyco.dev` relying party. So the app holds a P-256 key in the Android Keystore, unlocked per use by the fingerprint or screen lock (COI-2623). The plugin is [`aterm-android/plugins/tauri-plugin-devicekey/`](../../../../aterm-android/plugins/tauri-plugin-devicekey/). The page calls it as `window.__TAURI__.core.invoke("plugin:devicekey|<command>", args)`, which needs `withGlobalTauri` and no client dependency.

**Only Kai's own terminal grants typing.** The enrollment code is minted by `aterm passkey enroll` in a terminal of her own, which refuses inside a session. The plugin holds the key and the daemon decides. `aterm passkey revoke` forgets device keys too.

## Commands

Every binary value is base64url without padding. A rejection is `{code, message}` with `code` one of `cancelled`, `lockout`, `none_enrolled`, `unsupported`, `invalidated`, `other`. The client words each code and never shows `message`.

* **`status()`** - never prompts, cheap - `{enrolled: bool, key_id: string or null, biometric: "ready" | "none_enrolled" | "unsupported" | "invalidated"}`. `none_enrolled` also covers a phone with no screen lock. Call it before sending an enrollment code, because the daemon spends the code at `device_enroll_begin`.
* **`enroll({challenge})`** - no prompt - `{key_id, public_key, attestation: [string]}`. `challenge` is the daemon's 32 raw bytes and goes to the Keystore as the attestation challenge. `public_key` is the SubjectPublicKeyInfo DER. `attestation` is the certificate chain, leaf first. `key_id` is the base64url of SHA-256 over the SPKI. It replaces any earlier key, and it generates in StrongBox when the phone has one.
* **`sign({challenge, prompt})`** - one BiometricPrompt - `{key_id, signature}`. `prompt` is an optional title, 64 characters at most. `signature` is the ASN.1 DER ECDSA over SHA-256 of **M = "aterm-device-assert-v1" || 0x00 || challenge**. The plugin builds M, so a page script can sign nothing else. The daemon checks it with `ecdsa.VerifyASN1(pub, sha256(M), signature)`.
* **`forget()`** - deletes the local key, returns `null`. It does not revoke on the daemon.

`challenge` must be base64url, 16 to 64 bytes. Anything else rejects with `other`.

## What the key is

The key is non-exportable and bound to the user for every signature, with no grace window: `setUserAuthenticationRequired(true)`, timeout 0, `BIOMETRIC_STRONG` or device credential. The framework `BiometricPrompt` takes it through a `CryptoObject`, so the plugin adds no androidx dependency. minSdk is 30, the first release where that combination works. The daemon checks the attestation chain instead of trusting the claim.

## Wiring in the app

The plugin crate is a path dependency of `aterm-android/src-tauri`. The app needs four lines, then `bundle.android.minSdkVersion` of 30.

* `Cargo.toml` - `tauri-plugin-devicekey = { path = "../plugins/tauri-plugin-devicekey" }`
* `lib.rs` - `.plugin(tauri_plugin_devicekey::init())`
* `tauri.conf.json` - `"app": {"withGlobalTauri": true}`
* `capabilities/default.json` - grant `devicekey:default` to the main window

## Checked and not checked

* **Checked** - the Kotlin compiles against the Tauri Android API and its 6 unit tests pass on the JVM. The Rust crate type-checks for `aarch64-linux-android` and its 6 tests pass on the host. A JVM-made DER signature over M verifies in Go and one over the bare challenge does not.
* **Not checked** - anything on a phone. No Keystore key was generated, no prompt shown, no attestation chain read. Whether StrongBox generation, per-use auth with device credential, and invalidation on a new fingerprint behave as written is open until a Pixel runs it.
