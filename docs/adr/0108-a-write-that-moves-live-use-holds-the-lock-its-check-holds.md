---
status: accepted
---

# A write that moves Live use holds the lock its check holds

> **Amended by [[0140-a-write-that-arms-live-use-refuses-a-row-already-discarded]]:**
> each Live use reader gains its inverse, called under the lock's type by every write that moves
> Live use.

> Refines [[0099-live-use-is-what-will-still-send-and-upkeep-skips-a-deleted-row]]: it listed the
> writes that take the Workspace lock; this defines them and makes the lock part of their types.

## Context

ADR 0099 puts a removal's check of Live use and the writes that create Live use under one
transaction-scoped advisory lock of the Workspace. The writes it lists are scheduling a Broadcast,
Publish, and saving a Segment or a Rule. The check side is uniform: every removal opens
`WithinWorkspaceLockTx` and refuses with a `liveuse.Error`.

The write side is a list in prose, and it misses writes:

- Making an Integration its Channel's default moves every send that names no Integration onto it.
- Changing an Integration's config can change the From its sends go out with, and so the Sending
  domain they use.

`integrations.Update` makes both changes in a transaction without the lock. `integrations.DeleteIn` is
handed a row read before the deletion's transaction. Suppose Integration Y is promoted while its
deletion runs:

1. The deletion reads Y as not the default and finds no Live use.
2. The promotion commits.
3. The deletion discards Y.

Y is left discarded and marked default. Its Channel has no usable default, and a live Broadcast holds
with `no_integration`. ADR 0099 rejected exactly this outcome ("No lock").

Nothing in the code tells such a write from any other. `WithinWorkspaceLockTx` hands over the same
`*ent.Scoped` as an unlocked transaction, so a reader of a Live use check, a removal, or a write that
moves Live use cannot see whether the lock is held. The hot paths ADR 0099 leaves to a row lock (a
Rule firing, an enrollment) rely on the same convention: `contactlock.Lock` and `FOR UPDATE` return
nothing that the following write needs.

## Decisions

- **A write moves Live use when, after it, live work uses a different row.** That covers ADR 0099's
  list and adds:
  - making an Integration the default of its Channel, on create or on update;
  - changing an Integration's config so that the From its sends go out with changes.
- **The Workspace lock is a type.**
  - The bus hands a scope type of its own only inside `WithinWorkspaceLockTx`. `scopegen` generates it
    beside `*ent.Scoped`.
  - Only that type is accepted by:
    - the readers of Live use (`UsingTag`, `LiveSends` and their kin);
    - the unchecked removals behind a Live use check (`x-unchecked-removals`);
    - the writes that move Live use.
  - A forgotten lock does not compile, and the list in prose becomes the set of signatures that take
    the type.
- **A row lock is a value.**
  - Locking a row on a hot path (`contactlock`, an Automation or a Rule locked `FOR UPDATE`) returns
    a value that stands for the locked row.
  - An enrollment and a Rule Action take that value, so ADR 0099's two locks are both held by the
    compiler.
- **A removal reads its row under the lock.** `integrations.DeleteIn` takes the id, reads the row
  inside the locked transaction, and clears the default in the same statement that discards it.

## Considered Options

- **A list of functions in `easement.yml` that only a lock holder may call.** It matches names, not the
  transaction, so a function that opened its own unlocked transaction would pass.
- **Keep the prose list and add race tests.** A test covers the writes someone thought of. The missed
  Integration edit is one no test covered.
- **Lock every row a Live use check reads.** ADR 0099 already rejected this: a Sending domain is
  referenced by the domain of an address, not by a key.

## Consequences

- `integrations.Update`, `integrations.Create` (as default) and their callers move into the locked
  scope. The Integration and Sending domain deletions' readers take it.
- The race above gets one test on a real commit (`testhelper.RealDB`). It shows that the promotion
  waits for the deletion.
- The Workspace lock paragraph of `docs/architecture/lifecycles.md` lists the type instead of the writes, with
  the code.
