# Feedback from the screen

A person takes a screenshot of the aterm client, taps one button, and sends it with a note. The host keeps both as a record and a frontend-eng seat files it. Kai asked for it on 2026-11-04 after about a dozen cases in an hour on the Android app.

**The sheet is the client half, and the daemon's `feedback` frame is the other (COI-3345).** The sheet shows only where the connection has a `FeedbackChannel`, which `DaemonHost` gets once the daemon lists the `feedback` feature. Until then a real host's menu entry reads "This host needs a newer aterm daemon", and only the Demo host sends.

## What a person sees

* **On a phone, one tap.** A chat-bubble icon sits in the bar beside the seat switcher, 44px, and is absent while the host cannot take feedback. The sidebar and the phone menu carry a labelled entry that says why it is unavailable instead of vanishing.
* **The sheet opens over the screen being discussed** and names it in a line: the seat, the viewport, the Android app or browser, and the build. That line is read when the sheet opens, since the screen underneath is the subject. The build is the short git sha stamped at build time (`__ATERM_BUILD__`, `dev` outside a repo).
* **A note and up to four screenshots.** Add screenshot opens the phone's picker, which lists the newest screenshots first. A paste or a drop works on a desktop. A still image over the host's limit is shrunk, a GIF is refused, as in the composer.
* **A refusal keeps everything.** The note and screenshots stay, with the host's reason in words. Closing the sheet keeps the draft until a send succeeds.
* **A send names its record**, and Send another starts clean.

## Why this shape

A host's disk holds the files, so no bucket or credential reaches a phone. A seat files the tracker record, so the screenshot arrives where an engineer already reads. Jev chose images as host files for seats in COI-3239 and this reuses that rule: signature typing, 3,000,000 bytes, a daemon-named file.

**Not built yet.** Taking the screenshot from inside the app (Android's PixelCopy through a Tauri plugin) and receiving one from the system share sheet are native work in `aterm-android/`, COI-3359.

## Test

`just aterm-client-drive` opens the sheet at 1280 and 320px: the icon with no menu, the 44px target, a refused screenshot keeping the draft, the sheet fitting the screen, a kept draft, and a named record. The Playwright test bed drives the daemon's wire and has no journey for this until `DaemonHost` sends the frame.
