---
status: accepted
---

# A Segment's Condition compiles to generated ent predicates; an Event links to its Contact by a nullable FK

> **Amended by #749:** an Event test is its own `event` leaf (`action`, `operator` atLeast or
> atMost, `times`, optional `withinDays`) that counts the Contact's Events, not a `rule` leaf on
> `event:<action>` compiled to `contact.HasEventsWith`. It compiles to a subquery that counts the
> Events linked by `contact_id`, built with ent's builder and the generated Event predicates.

> **Amended by #750:** a `number` leaf (`gt`, `gte`, `lt`, `lte`) on a number Custom field, a
> `date` leaf (`before` or `after` a moment `at`, which split time at it) and a `dateWithin` leaf
> (the last N days up to now) on a datetime one compile to a Postgres jsonpath filter in silent
> mode, a raw fragment: ent's JSON comparisons cast the value, and one Contact's value of another
> type would fail the whole query, where the filter is false. The key, the filter and its operands
> are bound arguments. Only a date-time with an offset is a moment, so a value is never read in the
> server's time zone; a trailing Z is written +00:00 first, since jsonpath reads an offset only as
> ±hh[:mm] on Postgres 14. Each leaf takes only a Custom field of its type, and a `rule` leaf on a
> Custom field only the operators of its type, a table the compiler holds once and the API lists as
> each Custom field's `ruleOperators`.

A Segment's Condition is compiled into ent's generated predicates (`contact.EmailEQ`,
`contact.HasTagsWith`, `contact.HasEventsWith`), not into SQL assembled from table and column
names. The compiler takes the Workspace's scoped client, so a Condition can only name what that
Workspace has: a core attribute, a Tag, an Event action, or a Custom field of its catalogue
([[0006-custom-fields-not-traits]]).

The one dynamic piece is the key of a Custom field inside the `custom_fields` JSON column, which
no generated predicate addresses. It is a bound argument, never part of the query text: a key is
data from ingest (declared-by-use), so it can look like anything.

Reaching Events through a generated predicate needs an edge, so `Event.contact_id` becomes a
foreign key to Contact: nullable (anonymous Events before Identify have none) and
`ON DELETE SET NULL` (an Event outlives its Contact, so deleting one only unlinks them). This
replaces the earlier "plain column, never an FK" rule.

The API carries the Condition as a typed, recursive schema (a group of rules and nested groups,
told apart by `kind`; operators and combinators are enums), not as JSON inside a string. The
stored definition is that tree's JSON, so the contract, the stored shape and the compiler read
one format.

## Considered options

- **Keep the SQL builder behind a column-name schema.** Rejected: it is hand-built SQL with the
  names as strings, it cannot know the Workspace's catalogue, and it let a Condition's JSON key reach
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
