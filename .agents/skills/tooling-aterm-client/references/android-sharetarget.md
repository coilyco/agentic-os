# Sharing a screenshot to a seat from the phone

Android's Share menu lists the app for images (COI-3485). A person opens a screenshot, taps Share, picks the app, and the app opens on a sheet that names the picture and asks which seat gets it. It reuses the composer's upload, so a seat reads it like any pasted image ([image-upload.md](image-upload.md)).

**What a person sees, worst first.** No host yet says so and Send stays off. A host that cannot take images, a locked host, a reconnecting link and a host with no seat each say so in a red line, with the picture still shown. Anything left out is named in words (a video, a HEIC, a fifth image, a file the sender took back). With one seat running it is already chosen, with several the seat on screen is, and otherwise Send reads "Pick a seat" and stays off. A failed send says why and keeps the picture and the words. Discard, Escape, or a tap on the dimmed edge drops the share, and the original stays where it came from.

**The plugin is `aterm-android/plugins/tauri-plugin-sharetarget`, beside the device key's.** Its Android manifest adds `SEND` and `SEND_MULTIPLE` for `image/*` to `MainActivity`, and the manifest merger matches the entry by name. `MainActivity` is `singleTask`, so a share to a running app arrives in `onNewIntent`. Kotlin reads each file the moment the intent arrives, because the sender's grant to read it ends with the task, and holds it until the page calls `take`, which hands over everything waiting and forgets it. The page polls `take` at launch and each time it becomes visible, so no event channel exists. A desktop build has no Share menu, and `take` answers empty.

**The plugin passes any `image/*` through, and the client alone decides what a seat reads.** `images.ts` keeps the one list (PNG, JPEG, GIF, WebP), and a HEIC shared from another app is named in the sheet. The plugin reads at most 25 MB per item and four items, and the client shrinks a still over the host's limit the way the composer does.

**A failed `take` is silent.** An app built before the plugin rejects the call too, and a sheet that opened on every launch would be worse than a share that needs repeating. The Kotlin side catches each item's read error and reports it as a skipped item, so only an internal failure rejects.

**The sheet's send path is its own** (`share-send.ts`), a copy of the composer's upload then paste sequence, so it did not touch `Composer.svelte` while another change held it. They should become one function.

## Checked and not checked

* **Checked in the test bed** at 320px and 1440px, against the mock daemon with the app's bridge faked: the sheet, the seat choice by keyboard, the host that cannot take images, Discard, and the host receiving the picture's own bytes and the words.
* **Not yet checked on Android:** that the merged manifest lists the filters (read it with `aapt2 dump xmltree`), that Treehouse appears in the Share menu, that the picker Add image opens, and every step on a phone. [android.md](android.md) lists the open checks.
