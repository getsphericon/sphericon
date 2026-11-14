---
status: accepted
---

# The Send cap a Workspace starts with belongs to suspension

> Amends [[0067-the-send-cap-is-the-platforms-limit-the-provider-limit-the-customers]] (where the
> default Send cap lives), [[0102-the-core-is-layered-and-an-operation-across-modules-is-a-use-case]]
> (the Workspace creation composed twice) and
> [[0112-an-operation-belongs-to-the-module-that-owns-its-subject]] (the Sign-up and the Workspace
> creation kept as use cases for this read).

## Context

The Send cap of a Workspace is identity's: a column of `workspaces`, set by a Staff member through
`suspension.SetSendCap` and audited in the Workspace's Audit log. The default Send cap, the cap a new
Workspace starts with, was not: it was a column of the Instance state row, `instance_states`, which
`instancestates` owns, the top context. Every creation of a Workspace read it there
(`instancestates.Module.StartingSendCapIn`), on its own transaction, together with the Instance's
Send caps flag that `instancestates` was given.

That one read made creating a Workspace reach from identity, the bottom context, into the top one.
ADR 0112 kept the Sign-up and the Workspace creation as use cases for it (condition 1: the owner
would call a module above it), and ADR 0102's two compositions of the creation, in the Sign-up and
in the Workspace view, each made the read themselves.

The default is no setting of the Instance in its own right. It is the first value of a Workspace's
Send cap, a rule of the Workspace's standing beside the cap itself, and ADR 0067 states both rules
together.

## Decisions

- **`suspension` owns the default Send cap**, with the Send cap itself. It keeps the default in a
  one-row table of its own, `default_send_cap` (`ent/schema/default_send_cap.go`), whose row is made
  by the first change; without it no default is set. `suspension.DefaultSendCap` is given the raw
  client and the Instance's Send caps flag by the wiring, as a plain `bool`: identity sits below
  sending, where `providerlimit.SendCaps` lives.
  - `StartingIn(ctx, tx)` answers the Send cap a Workspace created now starts with, on the
    transaction that creates it: the stored default on an Instance with Send caps, none on one
    without them, whatever is stored.
  - `SetIn(ctx, tx, perDay)` sets or clears it on the caller's transaction under the row's lock,
    refuses a value outside `bounds.MaxPerDay` and any change on an Instance without Send caps
    (`suspension.ErrNoSendCaps`, in its own words, with the same message code as
    `providerlimit.ErrNoSendCaps`), and reports the change (the Audit diff value) and whether it
    changed; the same value, or clearing none, is a no-op.
  - `Get(ctx)` reads the default for the Staff console, and whether the Instance has Send caps at
    all: without them there is none to show.
- **The Staff's change of the default stays an operation of the instance context.**
  `instancestates.Module.SetDefaultSendCap` opens the transaction, calls `SetIn` and records the
  `instance.default_send_cap_change` entry in the Instance audit log, with the diff key
  `default_send_cap`, as before. `instancestates` imports `suspension`, which is below it, and no
  longer takes the Send caps flag itself: `suspension` is the one that knows it. The staff contract
  does not change.
- **`instancestates.Module.StartingSendCapIn` goes.** The Sign-up and the Workspace view ask
  `suspension.DefaultSendCap.StartingIn`.
- **A migration moves the stored default**: it creates `default_send_cap`, copies a default stored
  in `instance_state.default_send_cap` into its row and drops the column. An Instance with no
  stored default gets no row.

## Considered options

- **Keep the default in `instancestates` and hand the creation the value.** Each caller would still
  read it in the top context first, and the read would leave the creation's transaction.
- **Move the Staff's change of the default into `suspension` too.** The change is recorded in the
  Instance audit log, which `instanceauditentries` keeps in the instance context, above identity;
  `suspension` cannot write it.
- **Keep the default a column of the Instance state row, written by `suspension`.** The row is
  `instancestates`', and a table has one owner (ADR 0102, `entrywrite`).

## Consequences

- Creating a Workspace no longer reaches above identity for its Send cap. ADR 0112's reason to keep
  the Sign-up and the Workspace creation as use cases for this read no longer holds, though the
  Workspace view stays one for its reads of sending. Where the creation lives is weighed apart
  (#1359, #1360).
- The bound `MaxPerDay` is checked by `suspension` for the default (`boundgen`'s owners name
  `DefaultSendCap`), as by `workspaces` for a Workspace's cap.
- The migration drops a column, so its release is breaking (ADR 0033): the copy runs before the
  drop, in the same migration, and nothing is asked of a Self-hoster.
