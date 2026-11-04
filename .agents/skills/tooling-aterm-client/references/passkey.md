# Passkey unlock

A device on another machine types only after a passkey assertion with user verification. The frames and the daemon half are in [aterm-daemon.md](../../../../docs/aterm-daemon.md). This page is the client's side.

## What a person sees

* **Locked** - `welcome.typing` says `passkey_required` with `passkey` `enrolled` or `unenrolled`. The composer, terminal, choice cards, and launch buttons say read only. A `typing` push lifts it, and focus returns to the composer.
* **Standing unknown** - a refusal that carries no standing shows both the unlock button and the code steps, so a phone is never left with neither.
* **Unenrolled** - three steps and a code field. The code comes from `aterm passkey enroll` in a terminal no session started.
* **Enrolled** - one button, Unlock with passkey. A new connection asserts again unless a resume proof restores it (below).
* **Reachable unlocked** - on a hosted build the host page always carries "Set up a passkey with a code", whatever the lock says.
* **A refused send** - input has no reply, so a lock within 15 seconds of Send is that send refused. The text returns as the draft, and the lock panel says it was not sent.
* **A failure** - the daemon's own words for a wrong code, or a sentence for what the browser reported.

## Resuming after a redial

The daemon holds an unlock per connection, so a redial starts locked. When the welcome lists `resume`, the asserted reply carries a one-use `resume` proof, and the client sends `resume` on the next welcome before anything else. `resumed` hands back the next proof and the daemon pushes `typing` allowed. `resume_ended` is one reason for every failure and leaves the lock, so the person sees "Your unlock ended while this device was away" above the Unlock button. The proof lives in the host object's memory, never in storage or an event. A link that dies mid-resume keeps it. Without `resume` in the welcome nothing is sent and the proof is dropped. The window length is the daemon's flag (COI-3305).

## Why it is shaped this way

* **Only the hosted build offers the ceremony**, and the Android app does not. The relying party is `coilyco.dev`, so the daemon-served page cannot assert. It shows where to go and sends nothing.
* **The daemon spends the code at `passkey_enroll_begin`**, before it returns options. The client checks the browser can run a ceremony first, and a failure after begin says the code is used up.
* **Cancel on an ask is refused for a locked device**, so it stays disabled there, unlike a loopback read-only browser.
* **A read-only terminal lets Tab leave.** xterm traps Tab, and the unlock button sits after the terminal.

Open: Chrome's Local Network Access check blocked an https page from a loopback socket in testing. A tailnet host may prompt for permission the same way. Not seen on a real tailnet.
