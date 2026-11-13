# A client sees every primary key sealed under the Instance's id key

The APIs named a row by its bigint primary key, which comes from a sequence. A client that saw two
ids learned how many rows the Instance created in between, every tenant's together, and an id was a
permanent contract that said when a row was made. Workspaces already had an opaque public id (ADR
0043); nothing else did.

Every primary key a client sees is now its **public id**: the key sealed with AES under the
Instance's **id key**, as one block holding the key beside a fixed tag, written as 22 base62
characters (`internal/kernel/publicid`). The contract's `EntityId` is that string, and ogen reads
and writes it through `publicid.ID`, a key whose text is its public form, so handlers and modules
keep working on keys. A string of the right form that is not sealed under this Instance's key fails
its tag and is refused as malformed (400) before any handler runs, like a string of the wrong form.

- **The id key is the Instance's, in the database.** 16 random bytes on the Instance row, made with
  it and never changed: a restored database keeps every id, an operator holds no new secret, and a
  database reader already sees the keys themselves. It never rotates, since a new key would change
  every id a client has stored.
- **Derived, not stored.** A public id is computed from the key, with no column or lookup, so a
  reference (a Broadcast's `segmentId`) costs nothing to show, and the id of a row already gone (an
  Erasure's Contact, a `contact.erased` webhook) is as readable as any other.
- **Every exit, not only the contract's fields.** The id key seals the keys a client meets outside
  the typed contract too: a webhook body (its `contactId`, every top-level numeric `…Id` of the
  event's data, and the Workspace by its public id), a System event's key properties
  (`systemevent.KeyProperties`) wherever an Event is shown or bound into a template, the targets,
  actors and reference diffs of Audit entries (stored in the public form, so a client filters by
  what it was shown), the cursors of the Audit log and the Export streams, and the ids in a signed
  link token, which a recipient can read. A test marks the Contacts' keys with a number nothing else
  holds and finds it in no response, log, stream, cursor or delivery.
- **What stays internal.** The service API's feed positions, read by the operator's control plane,
  and a platform identity's id, which the Audit log masks as Staff anyway.

Consequences: equal keys of two tables have equal public ids, which tells a client nothing it can
count. A public id says nothing about a row's age, so the order of a list is the API's to give. An
id a client stored before this change names nothing: the app had no clients yet.

## Considered options

- **A random `public_id` column per exposed table** (rejected): every response would load the public
  id of each row it references, and a deleted row's id could not be shown at all.
- **Time-ordered primary keys** (rejected): low bits from a shared sequence still count other
  tenants' rows; a leak-free layout needs a leased node number per process and caps the processes
  under the 2^53 that ids travelling as JSON numbers allow.
- **Accept the leak** (rejected): enumeration across tenants was already blocked by the scoped
  client, but the instance-wide volume and the creation order were not.
