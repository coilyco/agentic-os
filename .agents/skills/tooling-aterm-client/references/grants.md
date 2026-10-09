# Approval grants

`aterm grant` carries one decision Kai made to the seats that execute it, so she types it once, not once per seat per step. It does not weaken a wall. A gate accepts a grant only for the exact action it was made for.

## The contract

* **Request** - `aterm grant request` takes an exact `--action`, an exact `--target`, one or more `--gate` names, one or more `--for` roles, a `--ttl` of at most 12 hours, and `--uses` (1 to 10, default 1). Anything vaguer fails before Kai sees it.
* **The question is the daemon's** - it renders the card from those fields and stamps the asker from its token. A seat cannot show one thing and bind another. Approve is option 0, and there is no free text.
* **Only her answer mints** - the grant is made inside the `answer` path, past the same typing guard that refuses a session descendant, an unverified tailnet device and an unlisted browser. A `send`, a relay, a stamped director message or a file cannot mint one. The asker gets the id back in its answer.
* **A relay carries the id, not the authority** - the id is opaque. The daemon holds the record, so a director saying "Kai approved" proves nothing and a director holding an id it was not named for is refused.
* **Check** - `aterm grant check <id> --action --target --gate` asks the daemon. It prints `allow, N uses left` or `deny: <reason>` and exits 1 on a deny. A refusal never spends a use. An allow spends one.

## Refusals, in order

* `unknown_grant` - no such id, or it expired and was dropped, or the daemon restarted. A restart voids every grant, which fails closed.
* `no_provenance` - the record was not made by an answer.
* `wrong_holder` - the presenting seat's role is not in `--for`. The role comes from the token, never the frame, and this check runs before expiry and use count so a seat the grant does not name learns nothing of its state.
* `expired`, `spent` - past its time, or all uses taken.
* `wrong_action`, `wrong_target` - not an exact match. A prefix does not match.
* `gate_not_named` - the gate asking is not one the record named.

## What a gate does

A gate that today demands Kai's word in its own window runs `aterm grant check` first and proceeds on exit 0. On a deny it shows the reason and stays refused, and the seat's unrelated work carries on. The gate names itself and states the exact action and target it is about to run.

## What it does not do

* **Harness prompts and classifier refusals** - these are decided by the harness a seat runs in, not the daemon. A grant does not answer them. Their values belong to the Access Sysadmin seat once a gate uses this contract.
* **Persistence** - grants live in the daemon's memory.
* **Destructive, credential, publication, merge and force walls** - they stay where they are. A grant only answers a gate that names it and only for the exact action Kai approved.

## Measured

`TestReplayOfTheLaunchNightSequenceNeedsOneAnswerPerDecision` replays the gates in COI-2664 that accepted only her in-window word: the wipe, go-live, the spawn-check swap (twice), the Echo deploy, the Steam branch switch and the desktop click. That is 7 typed approvals before and 6 decisions now, with 10 gate steps run on them. The rest of the reported 20 were harness prompts and classifier refusals, which this does not reach.
