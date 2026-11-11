# A seat's artifact in the client

A seat can put a claude.ai artifact in front of Kai without her leaving aterm. claude.ai refuses to be framed (`frame-ancestors 'self'`, measured in COI-3325), so the [host daemon](../../../../docs/aterm-daemon.md) pushes the seat's own source file and the client draws that, with a link out to the page. This page is the daemon half (COI-3329) and the client's reader (COI-3330).

**`aterm artifact add --url <page> --title <name> --source <file> [--waiting] [--standing]` registers a page.** `aterm artifact waiting <url|id> [--off]` and `aterm artifact remove <url|id>` change it. The MCP tools are `add_artifact`, `set_artifact_waiting` and `remove_artifact`. The seat is named by its `ATERM_SESSION_TOKEN`, as `send` is.

**The daemon reads the file, not the verb.** The request carries an absolute path, so the check cannot be skipped by a client that sends bytes.
* The address must be exactly `https://claude.ai/artifact/<id>` (`bad_url`). The id becomes the page's own.
* The path must resolve, symlinks followed, to a regular file under the seat's spawn working directory, its session shadow root, its `TMPDIR`, or `claude-<uid>` under `/tmp` (`bad_path`). A root of `/` admits nothing. `claude-<uid>` is the harness scratchpad and is shared by every claude seat of this user, since the daemon cannot name one seat's. A seat adopted after a daemon restart has only its working directory and that scratchpad.
* At most 2 MiB (`too_large`), checked on the stat and again on the read. It must be UTF-8 (`not_text`). The title is 1 to 200 bytes with no control character.
* At most 16 pages per seat and 16 standing in all (`too_many`).

## Frames

Welcome feature `artifacts`. They ride the `views` channel, replayed on `subscribe` and pushed live.
* `artifact` `{session, artifact: {id, url, title, waiting, standing, html, seat_ended?}}` is a whole page. A second add of the same address pushes it again and the client replaces by `id`.
* `artifact_update` `{session, artifact_id, waiting?, seat_ended?}` carries only what changed. A new title or source is a whole `artifact`.
* `artifact_closed` `{session, artifact_id}`.
* The requests are `artifact_add` `{token, url, title, path, waiting?, standing?}` answering `artifact_added` `{artifact_id, delivered}`, and `artifact_waiting` and `artifact_remove` `{token, artifact_id}`. `artifact_id` takes the address or the id. Refusals carry `reason`: `bad_url`, `bad_path`, `too_large`, `not_text`, `too_many`, `artifact_owned`, `no_artifact`.
* **The daemon sends no `csp` and no `live`.** The sandbox policy is the client's.

## When the seat ends

* `session` is the posting seat's name and stays so as provenance. Pool names recur, so a client must not read it as a live seat once `seat_ended` is true.
* A page that is not `standing` gets `artifact_closed`. A standing page gets `artifact_update {seat_ended: true}`, no close, and keeps its waiting state.
* **Ownership.** Only the seat instance that posted a page, named by its name and start time, changes it. Once a standing page's owner has ended, any seat may change or remove it, and a later add of the same address takes it over as a whole `artifact` with the new seat's name.
* **A restart keeps standing pages.** Pages are written to `artifacts/` beside the ledger (`~/.local/state/aterm/artifacts`, 0700 directory, 0600 files). After a restart a page whose seat was adopted comes back as posted, a standing page whose seat is gone comes back with `seat_ended`, and any other page is dropped. A file the daemon did not write is left alone.

**The source and the path are never logged.** The daemon logs the seat and how many clients took the page.

## The reader (COI-3330)

* **A list, "Pages for you".** At the top of the seat overview and of every seat's Views tab, waiting pages first. A row is the title, who posted it (`from Imp-Dragonfly, seat ended` once that seat is gone) and a Waiting chip. A host with no pages shows none, and its Views tab is unchanged.
* **A page opens in a full-screen reader** over whatever is on screen, drawn from the source the seat published it from. The frame is `sandbox="allow-scripts"` and never `allow-same-origin`, so a page runs with no origin and no access to this app's storage. The client pins a policy first that lets only the hosts the Artifact tool allows load (cdnjs, jsDelivr, unpkg, the Tailwind and jQuery CDNs, Google Fonts and its font files), with no network for the page itself. The daemon sends no policy. `artifact-pages.ts` holds the list and a test pins the sandbox word.
* **Open on claude.ai is the first action** after Back. claude.ai refuses to be framed, so the real page opens in the device's own browser, where Kai is signed in. Measured on the Android app: the opener launches Chrome, and Back returns to Treehouse with its state kept. Only an address that is exactly a claude.ai artifact is offered.
* **A page that calls `window.claude`** says "Answer on the page", since a copy here cannot save an answer. A page whose seat ended says so, and a page its seat closes says so and leaves the list. A page with no source says so and still offers the real page.
* **Attention.** A waiting page already counted as needing Kai (COI-3342). Its toast now opens the page, not the seat that left it. Focus moves into the reader and back to its row, Escape closes it, and the page behind is inert while it is open.
