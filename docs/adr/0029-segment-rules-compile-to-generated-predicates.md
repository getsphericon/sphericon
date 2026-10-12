---
status: accepted
---

# Segment rules compile to generated ent predicates; an Event links to its Contact by a nullable FK

A Segment's rule query is compiled into ent's generated predicates (`contact.EmailEQ`,
`contact.HasTagsWith`, `contact.HasEventsWith`), not into SQL assembled from table and column
names. The compiler takes the Workspace's scoped client, so a rule can only name what that
Workspace has: a core attribute, a Tag, an Event action, or a Custom field of its catalogue
([[0006-custom-fields-not-traits]]).

The one dynamic piece is the key of a Custom field inside the `custom_fields` JSON column, which
no generated predicate addresses. It is a bound argument, never part of the query text: a key is
data from ingest (declared-by-use), so it can look like anything.

Reaching Events through a generated predicate needs an edge, so `Event.contact_id` becomes a
foreign key to Contact: nullable (anonymous Events before Identify have none) and
`ON DELETE SET NULL` (an Event outlives its Contact, so deleting one only unlinks them). This
replaces the earlier "plain column, never an FK" rule.

The API carries the rule tree as a typed, recursive schema (a group of rules and nested groups,
told apart by `kind`; operators and combinators are enums), not as JSON inside a string. The
stored definition is that tree's JSON, so the contract, the stored shape and the compiler read
one format.

## Considered options

- **Keep the SQL builder behind a column-name schema.** Rejected: it is hand-built SQL with the
  names as strings, it cannot know the Workspace's catalogue, and it let a rule's JSON key reach
  the query text (ent writes a Postgres JSON path key unescaped).
- **Keep Events FK-free and write one correlated `EXISTS` by hand.** Rejected: it is the last
  hand-built piece, and the FK costs little: a primary-key lookup per insert and an indexed
  update on Contact deletion (the `(workspace_id, contact_id, action)` index covers it).

## Consequences

- `events.Persist` runs after the fact, so the Contact may be gone (erased, deleted) or not this
  Workspace's. It then writes the Event unlinked instead of failing on the FK, which would
  otherwise retry the bus message forever.
- Erasure ([[0021-gdpr-erasure-and-export]]) is unchanged: it deletes or anonymizes the Events
  explicitly before the Contact goes, so `SET NULL` never leaves a customer Event behind.
- `ScopedRef{Unchecked}` is gone from `contact_id`: through the edge, the scoped client checks
  that a linked Contact is in the Workspace.
- A rule naming a Custom field the Workspace does not have is invalid when the Segment is saved
  and when a Broadcast resolves it, instead of silently matching nobody.
