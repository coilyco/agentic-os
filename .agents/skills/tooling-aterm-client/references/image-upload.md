# Handing a seat an image

A browser, possibly on a phone, cannot put a file where a seat on the host can read it, so the [host daemon](../../../../docs/aterm-daemon.md) takes the bytes and writes the file. The client then types the path into the seat with `input`. Jev chose this shape over a clipboard bridge and terminal graphics, which no harness reads from stdin (COI-3239).

**`image_upload` carries a `session` and base64 `data`, and `image_uploaded` answers with an absolute `path`.** The `id` is echoed. The welcome feature is `image-upload`, offered only by a daemon that has the build, so a client checks it before sending megabytes.

**The daemon reads the type from the bytes and never from the client.** PNG, JPEG, GIF and WebP signatures are the whole list, and anything else is `not_an_image`. A `mime` the client sends is ignored. The daemon names the file (random name, extension from the signature), so no name or directory comes from the client.

**The cap is 3,000,000 bytes, set by the frame and not by taste.** A frame is one JSON line capped at 4 MiB, base64 is 4/3 the size of its bytes, and a longer frame drops the connection before the daemon can reply. `too_large` is checked before anything is written. A client larger than the cap resizes first. A seat holds 16 files, then `too_many`.

**Files live in `images/<session>-<start>/` beside the daemon socket.** The socket directory is the one place every seat that reaches the daemon shares, which a session shadow's `HOME` or working directory is not. The directories are 0700 and the files 0600, so only the user reads them. The start time keeps a recycled pool name from inheriting an older seat's files.

**The files go when the session does.** Ending a session removes its directory. A daemon start removes every directory that no adopted session owns, so a restart that adopts live seats keeps their images.

**It takes the typing guard.** A tailnet device needs a passkey first (`passkey_required`), and a caller inside a session is refused (`session_descendant`), the same as `input`. The session must be a seat this connection is attached to, else `session_unknown`, and a plain terminal is not a seat.

**No bytes reach a log or Sentry.** The daemon logs the session, the type and the size.

## What each harness does with the path

Observed on 2026-11-04 against a private daemon on this build. Each seat started in a directory outside `images/`, was handed one 11.9 KB PNG reading `ZEBRA 4417`, and was asked what it said. The path was sent twice, as plain `input` keystrokes and as a bracketed paste (`ESC[200~` path `ESC[201~`).

* **claude** - a paste becomes an `[Image #1]` attachment and the model answered `4417` with no tool call. Typed keystrokes stay text, and the model opened the file with its Read tool and answered correctly.
* **codex** - a paste becomes `[Image #1]` and the model answered `4417`. Typed keystrokes stay text, and the model still answered `ZEBRA`. The screen does not show how it read the file.
* **opencode** - a paste becomes an `[Image 1]` attachment with a `File` row. The model configured at the time (DeepSeek V4 Pro, since removed in COI-3268) answered that it does not take image input, so the attachment arrived and the picture did not. The pin has moved, so test the current model before assuming it can or cannot see an image. A vision model is needed to describe it.
* **goose** - a paste stays path text, with no attachment. Its model (an Ollama model) opened the file through a tool and printed it as ASCII art, then answered `3`, which is wrong. The file was readable, the picture was not seen.

**So a client should paste the path, not type it, when the seat has bracketed paste on** (`bracketed_paste` on the `sessions` entry). The daemon passes both through `input` unchanged. A seat that was handed a path it cannot attach, goose and any non-vision model, reads the file as text at best.

**Every seat read a file outside its working directory.** The seats ran as the same user in the same `/tmp`, so the directory beside the socket was reachable. A harness that sandboxes reads to its working directory (not observed, claude ran with bypass permissions) would need the file moved there.
