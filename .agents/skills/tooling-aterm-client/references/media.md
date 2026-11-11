# Files a seat shares: the media viewer

A seat's images, video and other files open in a viewer over the seat (COI-3458). The daemon half is [media-daemon.md](media-daemon.md), so this page names only what the client does and relies on.

## What a person sees

* **A chip** above the composer while the seat has shared anything: `<seat> shared 3 files, 2 new` and a View button, a 44px row. Several frames in a row are one chip and a sequence, never a run of lines in the terminal.
* **A viewer** over the whole screen. An image is fitted to the screen and a Rotate button turns it in quarters, since the first camera frame came out upside down. A video plays from its own controls. Previous and Next step through the sequence, with `2 of 8` in the header, and the view follows the newest frame until the person steps back.
* **Anything else** (a PDF, a G-code file, an archive) gets a card with its name and size and a Download button. The file is read only when Download is pressed, in 2 MB chunks.
* **A file the host no longer has** says so in words, with Try again. It keeps the newest 32 a seat shares.
* **By keyboard:** arrows step, Escape closes, Tab stays inside the viewer, and focus returns to the chip. The controls are 44px targets that fit 320px.

## What it relies on

A welcome listing `media`. The client subscribes to the `media` channel, which replays the seat's shelf, then reads a file with `media_read {media_id, offset, length}` and `media_data {offset, size, eof, data}`. A frame is capped near 4 MiB, hence the chunks. A host without the feature shows no chip.

## Not in the first slice

Zoom and pan, a PDF or 3D viewer, thumbnails, and any way for the person to send a file back. Download works in a browser. **In the Android app it saves nothing** (checked 2026-11-10: the Download folder stayed empty, and the WebView has no Web Share or save plugin), so inside the app the viewer shows no Download and says the app cannot save files yet. A native save is COI-3593.

## Trying it with no daemon that shares files

The bed's mock scripts the frames (`shareMedia`). The Demo host shares five synthetic files from Imp-Dragonfly once `localStorage.setItem("aterm.demo-media", "1")` is set and the page is reloaded, which is how the real Android WebView shows the viewer before any daemon does. Checked there on 2026-11-10 at 412px: the chip, a 320 by 180 image rotated twice and kept inside the stage, an H.264 clip that decodes and plays with the native controls, and the file card. The journey is `media-viewer`.
