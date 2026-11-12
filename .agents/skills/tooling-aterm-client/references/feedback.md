# Feedback from the screen

A person takes a screenshot of the aterm client, taps one button, and sends it with a note. The host keeps both as a record and a frontend-eng seat files it. Kai asked for it on 2026-11-04 after about a dozen cases in an hour on the Android app.

**The sheet is the client half, and the daemon's `feedback` frames are the other (COI-3345, see [the daemon half](#the-daemon-half)).** The sheet shows only where the connection has a `FeedbackChannel`, which `DaemonHost` gets once the daemon lists the `feedback` feature (COI-3434).

**The menu entry says why a host cannot take it, by what the welcome named.** "This host needs a newer Treehouse." only for a build older than aos-v0.518.0, the first with `feedback`. "This host has nowhere to keep feedback." for a newer build, since the daemon offers the feature only with a state directory. "This host does not take feedback." when the build is unreadable. The app reads this from the `features` event, never from the connection, so it updates when the welcome lands.

**A retry costs one frame.** A screenshot's token is kept per image, so a refused note resends only the closing frame. A token the host let go (`image_expired`) sends the pictures again once.

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

`just aterm-client-drive` opens the sheet at 1280 and 320px: the icon with no menu, the 44px target, a refused screenshot keeping the draft, the sheet fitting the screen, a kept draft, and a named record. The test bed's `feedback-send` journey drives the daemon's wire at desktop and phone sizes: a send with a screenshot, a refused note then a retry, an older host and a current host with no state directory.

## The daemon half

**Offered as the welcome feature `feedback`**, only by a daemon with a state directory. A client checks it before it sends megabytes, as with `image-upload`.

**Images ride one frame each, then a closing frame carries the note.** Four screenshots cannot share a 4 MiB frame.

* `feedback_image` `{id, data}` answers `feedback_image_saved` `{id, image}`. `image` is an opaque token good for ten minutes on that connection, and a connection holds at most eight.
* `feedback` `{id, text, screen, images: [token]}` answers `feedback_saved` `{id, record}`. `record` is the report's id, `<UTC date>-<time>-<hex>`. The reply does not wait for a seat.
* **The context object is the key `screen`**, not `context`, since `context` already carries a seat's context meter on the same frame. Its fields are `{view, host, viewport: {width, height, ratio}, build, app, at}`. The daemon clips each string to 200 characters, strips control bytes, and never trusts any of it.

**The daemon reads the type from the bytes**, PNG, JPEG, GIF or WebP, and ignores the client's `mime`. Each image is at most 3,000,000 bytes, a report 4 images, and a note 8,000 characters (characters, not bytes). A report needs a note or a screenshot.

**Refusals, as `reason` on the `error` frame:** `not_an_image`, `too_large`, `too_many` (five images in a report, or nine waiting on a connection), `text_too_long`, `image_expired` (a token the daemon did not issue to this connection, or one past ten minutes), `passkey_required` and `session_descendant`. A refusal keeps the tokens, so a retry resends only the closing frame.

**The report lives in the state directory, `~/.local/state/aterm/feedback/<record>/`**, beside the session ledger and never beside the socket, since `images/` goes with its session and macOS clears `/tmp` daily. It holds `report.json` (0600) and `image-1.png`, `image-2.jpg` and so on (0600) in a 0700 directory. A restart keeps every report. It removes staged screenshots, whose tokens died with the daemon, and a directory with no `report.json`, which is a write nobody was told succeeded. Reports are never pruned.

**It takes the typing guard**, because it writes to disk and reaches a seat. A locked tailnet device gets `passkey_required`, and a caller inside a session gets `session_descendant`.

**A frontend-eng seat is handed it through the `send` queue**, stamped `[from aterm feedback]`, so `queued` and `held` apply. The body holds the note between markers, the screen line, each screenshot's absolute path, and the path of `report.json`, and asks the seat to file a COI record (label `agentic-os`) with the screenshots attached. The daemon files nothing.

* **No seat live:** the daemon asks for one with `launch`, the same as `aterm send --launch`. If none opens within three minutes the hand-off fails and `report.json` still has no `handed` stamp.
* **A later seat gets it:** each `frontend-eng` spawn, and each daemon start for a live one, hands over every report with no `handed` stamp. A start never launches a seat for old reports.
* **Delivered** puts `handed` and `seat` into `report.json`. Delivered means typed into the seat's input, which is not the same as filed.

**No note text and no image bytes reach a log or Sentry.** The log holds the record id, the note's size in bytes, and the image count.
