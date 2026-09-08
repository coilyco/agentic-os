# `aosguard ops teable-admin`

Teable schema administration. The second Teable surface and the only one that
writes schema: the records surface stays the hosted MCP, records-only.

## Why it is scripts rather than a declarative wrap

An allowlist can gate one call. It cannot re-read after a write and compare,
and on this instance that comparison is the whole point. Teable reports
success without doing the thing in several confirmed ways, so a 2xx here is
not evidence about itself.

Every mutating verb therefore **writes, re-reads through a separate request,
and asserts the stored object matches what was sent**, failing loudly when it
does not. The create response is the thing under suspicion, so it is never the
evidence.

## Verbs

    aosguard ops teable-admin create-field   <table> --spec <field.json>
    aosguard ops teable-admin list-fields    <table>
    aosguard ops teable-admin create-table   <base> --spec <table.json>
    aosguard ops teable-admin describe-base  <base>

`create-field` refuses unless **every** requested property survives the round
trip, naming the ones that did not. This catches the defect where unknown
field properties are accepted and silently discarded, so a field asked for
with five properties is stored with three and returns 200 either way.
`notNull` is worth watching specifically: it is the most defect-prone
attribute here, refused while the query layer contradicts it.

There is no delete-field verb anywhere in Teable, so **a refused create leaves
a stray field only the Teable UI can remove**. The refusal says so rather than
implying a rollback happened.

## What it refuses, and why by name rather than by absence

Absence reads as "unimplemented" as readily as "refused", and the guesses go
wrong in both directions. So both refusals are discoverable verbs that name
the defect that earned them and point at the substitute:

* **`convert-field`** — the one verb that destroys data while reporting the
  opposite, having emptied all 6,536 values in a column it declared required
  while returning 200 with `notNull` true. Do it in the Teable UI with an
  export in hand, and read the column back before trusting the response.
* **`delete-table`** — Teable has no archive verb for a table, so a delete is
  unrecoverable outside a restic PVC restore. Rename in the UI instead, which
  is reversible.

Neither reaches the network. They exit non-zero with the reason.

## The credential

The admin PAT resolves from SSM at exec time, the same shape `ops actions` and
`ops netlify` use, so it never sits in a caller's environment. It carries
`field|create`, `table|create`, `table|delete` and `base|update` across two
bases — breadth that is correct for an admin token and wrong to hold in a
shell, which is the reason this surface exists rather than a `curl` and a
`curlrc`.

## Not a workaround for projection

An earlier design fanned out one request per field, because `list_record`
projection returned 200 with every field emptied. That defect was a
serialization bug, since fixed: projection is declared as an array and reaches
Teable as repeated query keys, which is the shape it always wanted. **Use
projection.** An unprojected read of a 6,748-row table pulls every field of
every row to get the three you asked for.
