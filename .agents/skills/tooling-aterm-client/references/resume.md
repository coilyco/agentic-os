# Resume: an unlock that survives a phone lock

A phone lock drops the websocket, and the redial is a new connection that starts locked (COI-3305). The unlock is a daemon fact about one connection (`client.asserted`, `aterm/daemon.go`), and the Keystore key behind the Unlock button asks for a fingerprint on every signature. So each wake cost a tap and a prompt. A resume proof carries one unlock across the redial, until the client app closes, or for a window the daemon is told. The code is `aterm/daemon_resume.go`. Frames are in [aterm-daemon.md](../../../../docs/aterm-daemon.md).

## How it works

* **Issue** - a passkey enrollment, a passkey assertion, or a device key assertion unlocks a connection, and its reply (`passkey_enrolled`, `passkey_asserted`, `device_asserted`) adds `resume`, a random 32-byte base64url proof, and `expires_in`, the window in seconds, which is absent when proofs last until the app closes. The daemon keeps only its SHA-256.
* **Present** - the welcome of a redial still says `passkey_required`, since the daemon cannot know a proof before a frame. A client that holds a proof and sees feature `resume` sends `resume {resume}` before it shows Unlock.
* **Accept** - a live proof answers `resumed {resume, expires_in}` with a replacement, then a `typing` push with `allowed`. Each proof is single use, so the client replaces what it holds every time.
* **Refuse** - an error with reason `resume_ended`. The connection stays exactly as locked as it arrived. Expired, spent, unknown, never issued, a daemon restart, and a `passkey revoke` all read the same, so a probe learns nothing.
* **The window** - by default there is none: a proof lasts until it is spent, revoked or the daemon restarts, and the client drops it with the page memory when the app closes (Kai's ruling on COI-3305, 2026-11-04). With a duration set, it runs from the moment the holding connection drops, and a connection still open never expires. A resume starts a fresh window, so it is an idle window and a chain of short gaps never asks for a fingerprint again.
* **In memory only** - a daemon restart re-locks every device. The client keeps the proof in page memory, never storage or logs.
* **When the client is gone** - in `app` mode the daemon cannot tell a closed app from a locked phone, since both are a dropped socket, so a dropped socket ends nothing and no absolute lifetime exists. A proof ends only by use (a replacement follows), `aterm passkey revoke` (the one way to remove a device key, which clears every proof), a daemon restart, or eviction as one of the 64 oldest closed. A proof copied off a live page is single use, so whichever copy redeems first wins and the other reads `resume_ended`. A bounded daemon takes `--resume-window <duration>`.

## What it leaves closed

* **A connection with no live proof** gets `passkey_required` on every locked frame and every typing frame (COI-2484, COI-2540). A refused `resume` changes nothing.
* **A remote device still cannot open a terminal**, proof or not (`remote_terminal`).
* **Only a remote web connection resumes.** Any other peer gets a usage error.
* **The Keystore key is unchanged**, timeout 0, so every signature still asks for a fingerprint. Raising it would loosen the key for every signature and works for the APK alone. A proof covers the PWA too.

## The setting

`aterm daemon --resume-window <app|0|duration>`, or `ATERM_RESUME_WINDOW`. The default is `app`: no idle timeout, and the daemon keeps at most 64 closed proofs, oldest first. `0` turns resume off and restores one unlock per connection. A duration such as `60m` bounds a proof from its connection's drop. A negative value is refused. The daemon logs the setting at start and `a remote device resumed its unlock` on each resume.

## Pixel procedure

Nothing below has run on a phone. Every result is **unverified** until Kai or a seat with a Pixel fills it in. The APK must carry the client half that presents the proof (COI-3305, frontend), and the host daemon must run this build.

1. **Host.** Restart the daemon on the build. Expect the log line `a remote device's unlock survives a reconnect until its app closes`. Result: unverified.
2. **Unlock once.** Open the app and tap Unlock with fingerprint. Expect one prompt, then a live composer. Result: unverified.
3. **Lock inside the window.** Lock the phone, wait 2 minutes, unlock the phone, return to the app. Expect no prompt, a brief resuming state, a live composer, and the log line `a remote device resumed its unlock`. Type one line and Send. Result: unverified.
4. **Repeat.** Lock and wake three more times with gaps of 1 to 10 minutes. Expect no prompt each time. Result: unverified.
5. **Past the window.** Restart the daemon with `--resume-window 2m`, unlock once, lock the phone for 3 minutes, wake it. Expect the Unlock button, one prompt on tap, and no resume log line. Result: unverified.
6. **The gate.** Before tapping Unlock in step 5, try the composer, a choice card, and Launch. Expect read only, nothing sent. Result: unverified.
7. **Restart.** Restart the daemon inside the window and wake the phone. Expect the Unlock button, since proofs do not survive a restart. Result: unverified.
8. **Revoke.** In a terminal no session started, run `aterm passkey revoke`, then wake the phone inside the window. Expect the enrollment steps, no resume. Result: unverified.
