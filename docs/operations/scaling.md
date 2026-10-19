# Scaling and tuning

This page covers the connection budget, why a connection pooler in front of Sphericon is not
recommended, and the retention settings that keep the database bounded. All defaults below are
the shipped ones.

## Connection budget

Each replica opens two separate pools to PostgreSQL:

| Variable                         | Default                                | What it bounds                                                                                                                                                             |
| -------------------------------- | -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `SPHERICON_DB_MAX_OPEN_CONNS`    | `15`                                   | The `database/sql` pool: API requests and the domain-event consumers. The four event consumer groups hold long transactions on this pool.                                  |
| `SPHERICON_DB_MAX_IDLE_CONNS`    | equal to `SPHERICON_DB_MAX_OPEN_CONNS` | Idle connections kept open. Must be between `0` and `SPHERICON_DB_MAX_OPEN_CONNS`.                                                                                         |
| `SPHERICON_DB_CONN_MAX_LIFETIME` | `30m`                                  | How long a connection is reused before it is recycled.                                                                                                                     |
| `SPHERICON_PGX_MAX_CONNS`        | `25`                                   | The pgx pool the job queue (river) runs on. It must cover river's worker slots (20 across the default, broadcasts and webhooks queues) plus `LISTEN` and runtime services. |

Keep the total across all replicas below the server's `max_connections`:

```
(SPHERICON_DB_MAX_OPEN_CONNS + SPHERICON_PGX_MAX_CONNS) x replicas + ~15% headroom <= max_connections - 3
```

The `- 3` reserves PostgreSQL's superuser slots (`superuser_reserved_connections`, default 3);
the headroom covers backups, `psql` sessions, monitoring and migrations.

With the defaults and two replicas: `(15 + 25) x 2 = 80`, about `92` with 15% headroom, which fits
under `97` on a server with the stock `max_connections = 100`. The defaults therefore assume
**at most two replicas against `max_connections = 100`**. For more replicas, raise
`max_connections` (each connection costs server memory) or lower the two pool sizes. Do not
lower `SPHERICON_PGX_MAX_CONNS` below the worker total.

## PgBouncer and other poolers

Do not put PgBouncer (or another transaction-pooling proxy) between Sphericon and PostgreSQL:

- The job queue (river) relies on `LISTEN`/`NOTIFY` to wake workers. Notifications are tied to a
  session, which a transaction-mode pooler does not preserve.
- The domain-event consumers (watermill) hold long-running transactions with `SELECT ... FOR
  UPDATE` on the outbox. A pooler that multiplexes server connections breaks that locking.

If a pooler is unavoidable, use **session** pooling mode only, which keeps one server connection
per client connection. There is no PgBouncer-compatible mode.

## Retention

Two pieces of data grow with traffic: the **Event** table and the domain-event **outbox**. Both
are pruned in core by background jobs. The policy is [ADR 0019](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0019-event-retention.md).

| Variable                                | Default | `0` means | What it controls                                                                                                                                                                               |
| --------------------------------------- | ------- | --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `SPHERICON_EVENTS_RETENTION_DAYS`       | `400`   | disabled  | Deletes **analytical** Events (opens, clicks, deliveries, customer-tracked actions) older than the window, measured from when the row was inserted. Runs daily at 03:00 UTC, in small batches. |
| `SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS` | `7`     | no floor  | Deletes nothing by itself: the minimum age of a domain-event outbox row before the prune job (every 10 minutes) may delete it.                                                                 |

Negative values are rejected at startup. Deleting an Event is irreversible, so choose the window
deliberately; to keep everything, set `SPHERICON_EVENTS_RETENTION_DAYS=0`.

### What is kept

- **Evidentiary Events are never deleted by age.** These are consent proof
  (`marketing.confirmed`), permanent bounces (`email.bounced` with a permanent bounce kind),
  complaints (`email.complained`) and unsubscribes (`email.unsubscribed`). They are the audit
  and dispute record. Suppression itself does not depend on them: it is a separate registry that
  survives Event deletion.
- **Broadcast reports are unaffected.** They read the Broadcast recipient rollup, not live
  Events, so old campaign results stay readable after their opens and clicks expire.
- **Deliverability rates** (bounce and complaint, ADR 0011) read a short live window, far inside
  the 400-day default.

### Segment look-back cap

A [Segment](../guide/segments) condition that looks back further than the retention window only
sees Events that still exist, so its results are capped by `SPHERICON_EVENTS_RETENTION_DAYS`. For example,
"opened an email in the last 500 days" behaves like "in the last 400 days" on a default
instance. This is a documented product limit, not a bug. Evidentiary Events are exempt from the
deletion, so conditions on them are not capped.

### Outbox pruning

The outbox is pruned separately and more aggressively. The job deletes rows in batches only when
they sort strictly below the lowest consumer-group cursor **and** are older than
`SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS`, so no consumer loses an unread message and at-least-once
redelivery still works. It skips a run if any known consumer group has no offsets row yet, and it
removes the offsets rows of retired groups so a stale group cannot pin the outbox. The `events`
table is the durable record, so an outbox row is safe to drop once every consumer has passed it.

### Job queue retention

River job retention is set explicitly and is not configurable: completed jobs are kept 24 hours,
cancelled jobs 24 hours and discarded (failed for good) jobs 14 days, so the table stays small
and failures stay inspectable.

## Disk

Disk follows the Event rate. With the default window, expect the `events` table to level off at
roughly 400 days of analytical traffic plus all evidentiary Events. Lower `SPHERICON_EVENTS_RETENTION_DAYS`
to cap disk and backup size sooner.
