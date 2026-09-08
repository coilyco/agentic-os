# `aosguard ops teable-personal`

Teable **records** over one pinned personal base. The other half of the Teable
split: `teable-admin` writes schema across bases, this writes rows in exactly
one and can reach no schema verb at all.

## Why a second module rather than a second guardfile

`teable-admin.kdl` binds `agentic_os.teable_admin`, which creates fields and
tables. Pointing a personal PAT at that module would hand it a create-field
verb it has no reason to hold, so the guardfile binds
`agentic_os.teable_personal` instead. Argv is already the real control, so the
separate module is defence in depth rather than the only wall, and it means the
personal token never has a module that can reach schema.

The two modules share one client: `teable_personal` imports `TeableAPI` from
`teable_admin` rather than carrying a second copy. Nothing in the release
derivation models that edge, because it reads `argv` and cannot see an import,
so both ship only because each has its own guardfile. A test in
`tests/test_guardfile_python_exec.py` asserts the bundle is closed under
first-party imports, so dropping either guardfile fails there rather than at a
caller's import.

## The base is pinned, and the caller cannot name one

`list-base` and `list-base-table` take **no base argument**. The base id
resolves from SSM at exec time, the same way the token does, so there is no
argument in which a caller could name a different one. Every verb that takes a
table id checks that table against the pinned base's table list before any
request carrying it goes out, and refuses with exit 64 when it is not there.

This matters because a PAT's scope is set at creation and is not visible from
the value. The scope was checked rather than assumed: the personal token
returns 403 `notAllowedBase` against the issue-tracker base, so it is genuinely
restricted. The pin is the second belt, held because a token's scope can be
widened later without anything here changing.

## Verbs

    aosguard ops teable-personal list-base
    aosguard ops teable-personal list-base-table
    aosguard ops teable-personal list-table-field  <table>
    aosguard ops teable-personal list-table-view   <table>
    aosguard ops teable-personal list-record       <table> [--projection F]...
    aosguard ops teable-personal get-record        <table> <record>
    aosguard ops teable-personal create-record     <table> --spec <fields.json>
    aosguard ops teable-personal edit-record       <table> <record> --spec <fields.json>

The verb set mirrors what the hosted Teable MCP mounts, so the two surfaces can
be reasoned about together, minus the one below.

## Writes prove themselves

`create-record` and `edit-record` write, then **re-read through a separate
request** and refuse unless every requested field value survived, reusing
`readback_mismatch` (65) from the schema surface rather than minting a second
taxonomy. A 2xx from this instance is not evidence about itself, which
`teable-admin.md` documents at length and this instance has demonstrated more
than once.

A refused `create-record` leaves the row in place, because there is no
delete-record verb to undo it with. The refusal says so and points at
`edit-record` rather than implying a rollback happened.

**There is no `--typecast`.** The MCP offers it; it is left off here because
coercion makes a stored value legitimately differ from the requested one, which
would turn the read-back assertion into a source of false refusals. Send values
already in the field's type.

**Use `--projection`.** It is declared as an array and reaches Teable as
repeated query keys. An unprojected read pulls every field of every row to get
the ones asked for.

## What it refuses, and the two walls holding it

**`delete-record`** is not mounted. A connections row is hand-authored data
with no upstream to rebuild it from, and Teable offers no undo or archive for a
deleted record, so the row is gone outside a restic PVC restore. The precedent
is `convert-field`, which emptied 6,536 values while returning 200.

Two independent walls, on purpose. The guardfile grants no `delete-record`
leaf, so aosguard denies it before any module runs. The module also registers
it as a refusal that names the defect and exits 64 without reaching the
network, so anyone running the module directly gets the reason rather than an
argparse error. Absence alone reads as "unimplemented" as readily as
"refused", and only the second wall speaks.

**Reopen condition:** Kai asks for it, or a real workflow needs it and a
soft-delete or archive field cannot serve instead.

## The credential and the base id

Both resolve from SSM at exec time, so neither sits in a caller's environment:
`/coilysiren/teable/api-token-personal` for the PAT and
`/coilysiren/teable/connections-base-id` for the base. The base id is an opaque
random string, which is why it lives in SSM rather than in this file or the
guardfile.
