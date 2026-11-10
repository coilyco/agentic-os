# A seat shows a file in the client

A seat reaches a client with an address (`open_url`) or a claude.ai page (`artifact_add`), and neither carries an image, a video or a file (COI-3458). `aterm show` does. The daemon copies the file, keeps it for the seat, and serves it to a client in chunks. The client half, the viewer, is frontend-eng's.

## The seat side

**`aterm show <path> [--title NAME]`, and the MCP tool `show_media`, send `media_show {token, path, title?}`.** The token names the seat as `send` does. The reply is `media_shown {session, media_id, media, delivered}`, `delivered` being how many clients took it. Each call is one item, in order.

* **The path** must be absolute and resolve, symlinks followed, to a regular file under the roots `artifact_add` allows: the seat's working directory, its session shadow root, `TMPDIR` and the harness scratchpad. Otherwise `bad_path`. An empty file is `bad_path`, and one over 64 MiB is `too_large`.
* **The type is read from the bytes**, never from the name or the client. `image` is PNG, JPEG, GIF or WebP. `video` is MP4 (an `ftyp` brand a browser plays as MP4) or WebM. Anything else is `file` with the mime `application/octet-stream`, QuickTime and HEIC included.
* **The daemon copies the bytes** into a directory of the seat's own beside the socket, 0700, files 0600, so a camera frame that overwrites `latest.jpg` becomes a new item and not a changed one.
* **A seat keeps its last 32 items within 256 MiB**, the oldest evicted. A seat may show 240 files a minute (`too_many`). Ending the seat removes its directory.
* **The name** is `--title`, 1 to 200 bytes with no control character, else the file's own name.

## The client side

**The welcome feature is `media`.** A client says it can show media with `subscribe {channel: "media"}`, and the daemon answers `subscribed` and replays every live seat's kept items, oldest first within a seat. Replays and live pushes can repeat an id, so a client ignores a duplicate `media.id`.

* **`media {session, media}`** is pushed for each new item. `media` is `{id, name, mime, kind, size, at}`, with `kind` one of `image`, `video` and `file`, and `at` in RFC 3339.
* **`media_closed {session, media_id}`** is pushed when an item is evicted, and for every kept item when the seat ends.
* **`media_read {id, media_id, offset, length}`** answers `media_data {id, media_id, offset, size, eof, data}`. `data` is base64, and `length` is at most 2,000,000 and defaults to that. `size` is the whole file and `eof` says whether this chunk ends it, on every chunk. `offset` is left out when it is 0. A read past the end answers empty `data` with `eof` true.

## Guards

**A seat's files can hold secrets, so the rule is the one `message` text follows** ([message-text.md](message-text.md)): only a client allowed to type may subscribe or read. A process inside a session gets `session_descendant`, and a locked device gets `passkey_required`, on `subscribe` and on `media_read`. A client locked at subscribe subscribes again when `typing` says `allowed: true`. Each push is judged again per client.

* **`media_unknown`** is an item the daemon does not hold, or that was evicted.
* **The ring is memory only.** A daemon restart removes every kept file.
* **No name, title, path or byte reaches a log.** The daemon logs the seat, the kind and the size.

## What it does not cover

* **PDFs and 3D files** arrive as `file`.
* **No persistence**, and no push to a client that is not subscribed. The viewer opens from the replay.
* **The rig's test daemon** needs a release that carries `media` before the viewer can be proved on the emulator. See [android-emulator.md](android-emulator.md).
