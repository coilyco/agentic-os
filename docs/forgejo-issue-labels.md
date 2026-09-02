# The issue-label guard

Why `aosguard ops forgejo issue create` refuses an issue that carries no
priority label and no autonomy label, and why it takes label names rather than
the numeric ids the API declares.

## The rule

Every filed issue carries one `priority/P0`..`priority/P3` and one
`autonomy/headless`, `autonomy/live-collab`, `autonomy/async-consult`, or
`autonomy/epic`. The verb shadows the generated `issue create` leaf and
enforces both (#1105).

The refusal happens while the inputs bind, before the create fires, so a call
that violates the rule creates nothing. It is not a rollback and there is no
half-filed issue to clean up.

Since inbox and website migrated onto the same two vocabularies
(coilysiren/inbox#392) one rule covers every repository this surface writes,
and no per-repo exemption is carried.

## Names, not ids

`--labels` takes label names. The `POST /issues` body declares numeric ids, and
those ids differ per organization while the names do not, so validating a name
needs no per-org id table and no lookup. The caller stops running
`org-label list` before every filing.

The labels are applied by a second call to the labels sub-collection, whose
body schema declares the id-or-name union that carries a name at all.

## Why the globs enumerate

The guard spells `priority/P[0-4]` and each autonomy value by name rather than
writing `priority/*`. The labels endpoint drops an unknown name **silently with
a 200**, so a loose glob would accept `priority/p2`, let the write through, and
apply nothing. The caller would read success and get an unlabelled issue.

Enumerating is what makes the failure loud.

## What the shadow carries through

Optional fields of the generated leaf are carried through: an omitted one is
left out of the request rather than failing the call (umbra#326).

Two are dropped. The deprecated single `--assignee`, because `--assignees`
supersedes it, and `--closed`, because an issue filed already-closed is not a
thing this surface should make easy.

## See also

- [`aosguard`](../.agents/skills/tooling-aosguard/references/aosguard.md) - the
  build contract and ownership of the surface this verb lives in.
