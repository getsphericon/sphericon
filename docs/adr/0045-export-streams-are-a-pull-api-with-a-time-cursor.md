---
status: accepted
---

# Export streams: a pull API with a time cursor, in Enterprise

A Workspace that keeps its own copy of its data in a warehouse such as ClickHouse reads it
through **Export streams** (GLOSSARY): `GET /api/export/{events,messages,contacts,erasures,audit}`,
Bearer api-token, NDJSON rows (one JSON object per line, readable as `JSONEachRow`). Webhook
endpoints keep their plain JSON; they push single events and are not a bulk transport.

## Decisions

- **Pull, not push.** The consumer owns its pace and its failures, and a new consumer back-fills
  through the same API from an empty cursor. A push destination (a managed ClickHouse or S3 sink)
  would put credentials, retries and a warehouse schema in this repository.
- **The cursor is `(updated_at, id)`, inclusive, for every stream.** Rows are not immutable: Identify
  stitches anonymous Events onto a Contact and Erasure anonymizes `email.*` Events and delivery
  rows, and a `created_at` cursor would never deliver those changes again. A changed row is
  delivered again; the consumer keeps the newest version per `id` (ReplacingMergeTree).
- **No lock, no position, no safe horizon.** A transaction that commits late can land behind a
  cursor a consumer already passed. We do not prevent it: delivery is at-least-once with no
  ordering across commits, and the documented consumer contract is to re-read a trailing overlap
  and deduplicate by `id`; the overlap is at least 10 minutes, because `updated_at` is the application
  clock at mutation time, not at commit, so long transactions (a Broadcast, an Erasure) and clock skew
  between instances set how far back a late row can land. This is the convention of incremental connectors (Airbyte resends when
  a cursor is ambiguous). `workspacefeed` serializes positions with an advisory lock; Events are
  written on every open and click, so that lock would throttle the hot path.
- **A removed row is announced, never silent.** Erasure deletes customer-tracked Events, so a cursor
  cannot show it. The `erasures` stream carries the keys Erasure matched on (`contact_id`,
  `subject_id`, `visitor_ids`, and SHA-256 hashes of the lowercased address and phone, never the
  address itself), so the consumer deletes the same rows. The hashes are computed before the
  `contact.erased` message is published, so no address sits in the outbox. Rows live for the
  instance's Event retention window (ADR 0019; kept for good when retention is off), the age at
  which the Events they refer to are gone from a fresh read anyway: a consumer absent for longer
  re-seeds from an empty cursor. Deletion by retention is not announced: the consumer's copy
  follows its own retention.
- **Streams:** Events; Outbound messages (delivery metadata and address, no body or subject, the
  basis of every send, unlike a Broadcast recipient); Contacts (columns, Custom fields as one JSON
  column, Tag names); Erasures; Audit entries. Custom fields are a column of the Contact row, so
  they already move its `updated_at`; Tags are an edge (a join table), but the Tag change is an
  update of the Contact by id, and ent's `UpdateDefault` on `updated_at` moves it in the same
  transaction as the join row, so no extra write exists (a test pins it). The operations are hidden from MCP (ADR 0016),
  like the Audit log: a bulk feed is not an agent capability.
- **Enterprise.** License feature `export` (`ee/export`) and an `export:read` token scope; the
  `audit` stream also needs the `audit` feature. Unlicensed, the operations answer 404 like
  `/staff`. This is a deliberate exception to ADR 0014's rule of gating org-shape, not product
  value: a continuous warehouse feed is scale infrastructure of a high-volume sender, while the
  data itself stays reachable in core (CSV contact export, per-Contact Data export, the API).
  The Erasure tombstone is written by an `ee/export` bus subscriber, so an unlicensed instance
  stores nothing for it.

- **A suspended Workspace keeps its export** (Workspace suspension freezes sending, not the
  Workspace's own data); a deleted Workspace's tokens stop authenticating, and so does its export.
- **The Audit stream is separate from the Audit log list**: the list is for a person (filters,
  newest first), the stream is for a machine (cursor, oldest first).

## Considered options

- **Webhook endpoint as the transport** (rejected): no back-fill, and an endpoint disables itself
  after persistent failure, which silently ends a warehouse feed.
- **Scheduled files to S3 (Parquet or NDJSON)** (rejected as the first transport): the common
  vendor pattern (Customer.io), but it adds a storage credential and a schedule; it can be added
  later on top of the same streams.
- **Per-Workspace position under an advisory lock** and **a read horizon on the oldest open
  transaction** (rejected): both remove the late-commit gap and cost write throughput or a
  lagging, harder-to-explain feed.
- **`created_at` cursor for Events** (rejected): misses stitching and anonymization.

## Consequences

- A consumer that does not deduplicate sees duplicates and old versions; the guide for
  consumers shows the ClickHouse table (`ReplacingMergeTree(updated_at)`, order by `id`).
- Every table behind a stream needs an index on `(workspace_id, updated_at, id)`, added by a
  migration that only adds (ADR 0033), built concurrently.
