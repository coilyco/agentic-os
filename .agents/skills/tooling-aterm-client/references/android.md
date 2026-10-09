# aterm as an Android app

The [aterm web client](../../../../aterm-client/README.md) wrapped as an Android app with Tauri 2 (COI-2623). The APK bundles the built client, so it never loads through the hosted page's sign-in gate, and it dials a daemon over the tailnet like any other client. It is sideloaded, never on the Play Store.

**The project is `aterm-android/`, outside `aterm-client/` on purpose.** `aos-cli-release.yml` fires on any `aterm-client/**` path because the daemon embeds that tree, so a Tauri CLI bump inside it would cut an aos CLI release. `aterm-android/` holds the Tauri CLI as its own pnpm package and `src-tauri/` with the Rust shell. `tauri android init` writes the Gradle project at build time from the pinned CLI, so nothing generated is committed.

**The app origin is `https://tauri.localhost`.** `useHttpsScheme` makes the webview serve the bundle from there, and the daemon's origin check refuses any non-https origin, so `https://tauri.localhost` goes in `ATERM_DAEMON_ALLOW_ORIGINS` on every host the phone dials.

**The app registers one plugin, the [device key](android-devicekey.md), and no IPC commands of its own. `csp` is null like the web build.** `withGlobalTauri` exposes the plugin to the page, and the one capability grants the `main` window `devicekey:default` and nothing else, so the page otherwise talks to the daemon over its own websocket. Tauri is pinned at 2.12.1. GHSA-7gmj-67g7-phm9 (CVE-2026-42184, origin confusion that lets a remote page invoke local-only IPC on Android) is fixed from 2.11.1.

## Build

* **`just aterm-android-toolchain`** installs JDK 17, the Android command-line tools (pinned by SHA-256), platform 37.0, build-tools 36.0.0, NDK 28.2.13676358, which links at 16 KiB pages by default and the `aarch64-linux-android` Rust target. It runs as root, as the CI container does.
* **`just aterm-android-build VERSION OUT`** builds the client with `VITE_ATERM_APP=1`, runs `tauri android init`, then `tauri android build --apk --target aarch64`. Only arm64 is built, which is every current Pixel. Tauri derives the Android versionCode from VERSION as major*1000000 + minor*1000 + patch, so a larger version always installs over a smaller one.
* **`just aterm-android-sign IN OUT`** signs with apksigner from `ATERM_ANDROID_KEYSTORE_B64` and `ATERM_ANDROID_KEYSTORE_PASSWORD`, and with a throwaway key when they are unset, then runs verify. Verify checks the v2 or v3 signature, the package name, arm64-only native code, 16 KiB page alignment of the native library, and the signer certificate against `ATERM_ANDROID_CERT_SHA256` when that is set, because an update signed with another key does not install over the last one.

The scripted steps are [`scripts/ci/aterm-android.sh`](../../../../scripts/ci/aterm-android.sh). Every Android pin is manual and lives at its top.

## Not yet verified

* An APK installed and opened on a phone.
* Whether Android System WebView honours `interactive-widget=resizes-content` for the keyboard.
* Gradle and Maven dependencies are not hash-verified. Tauri's generated project pulls the Android Gradle plugin and AndroidX from Google Maven and Maven Central with no dependency verification metadata.
