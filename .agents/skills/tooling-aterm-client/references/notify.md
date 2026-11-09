# The coordinator pings Kai

**Only a coordinator seat makes a sound or a desktop notification for Kai, and it does so by calling `notify_kai` (COI-3517).** Every other seat stays silent whatever it does, and keeps its cues in the seat list. This page is the daemon half. The client half plays the chime, raises the notification and opens the sender's seat, and is frontend-eng's.

## When to call it

* **A real decision or a result that needs her now.** A choice only she can make, a blocked release, a finished piece she asked to hear about.
* **Never routine status**, progress, a task that finished and can wait, or a question a card or an artifact can carry. A card or a waiting artifact is the right path for those, and they reach her without a sound.
* **One short line each.** The title is at most 60 characters and the body at most 140. The daemon drops control characters and cuts the rest.

## What the daemon enforces

* **The token decides.** `notify_kai` and the `notify` frame carry the seat's `ATERM_SESSION_TOKEN`. The daemon looks the seat's role up in the Agent Compose roster and requires `pinned_by_default`, the attribute the client pins by (COI-3346). No role name is compared. A roster the daemon cannot read refuses, so a failure never lets a seat through.
* **Refusals** carry `reason`: `not_coordinator`, `too_soon` (a second try inside 10 seconds, counted from the last try even when it was refused), `bad_text` (no title) and `roster_unavailable`. A refused notice reaches no client, no phone and no broker.

## The frames

* `notify` `{token, title, body}` from a seat answers `notified` `{session, delivered}`, `delivered` being the clients subscribed to `sessions`. Welcome feature `notify`. A remote device needs its passkey for it, as for `ask`.
* **To every `sessions` subscriber:** `notify` `{notify: {id, session, role, identity, title, body, at}}`. It is an event, so a subscribe does not replay it. A client plays it once per `id`, and a tap opens `session`.
* **To a phone:** Web Push with `kind: "notify"`, the same fields as [web-push.md](web-push.md), `tag` `aterm-<session>`, sent whether or not a page is attached.
* **To the Treehouse broker** it goes as `kind: "ask"` with the body as the question, which the broker words itself, because its event kinds are closed. A notice reaches the broker phone push with the broker's wording, not the coordinator's, until the broker contract grows a kind for it.
