# Operations

This section is for the person who keeps a self-hosted sphericon instance running. Start with
[Self-hosting](../self-hosting) to install and configure it; the pages here cover what comes
after: sizing, backups, upgrades and monitoring.

| Page                             | Read it when                                                       |
| -------------------------------- | ------------------------------------------------------------------ |
| [Backup and restore](./backup)   | You are setting up backups or rehearsing a disaster recovery.      |
| [Upgrading](./upgrading)         | You are moving to a new release or a new PostgreSQL major version. |
| [Monitoring](./monitoring)       | You are wiring probes, metrics, SLOs and Prometheus alerts.        |
| [Runbook](./runbook)             | An alert fired and you need to know what to check.                 |
| [Scaling and tuning](./scaling)  | You are sizing replicas and connections or tuning retention.       |
| [Security hardening](./security) | You are preparing for production or rotating secrets.              |
| [Telemetry](./telemetry)         | You want to know what the Instance sends, or turn it off.          |
| [Self-hosting](../self-hosting)  | You are installing, configuring or wiring health checks.           |

::: tip
Not keen on operating it? [Sphericon Cloud](/guide/cloud) runs, upgrades and backs up the same product for you.
:::

## What you operate

sphericon is one process (the Go binary or Docker image) plus **one PostgreSQL database**. There is
no Redis, no object storage and no separate worker. That shapes everything below:

- **PostgreSQL holds all state.** Contacts, events, Outbound messages, provider credentials
  (encrypted), the background job queue (river) and the domain-event outbox (watermill) all
  live in the same database. A single database backup therefore captures the whole instance.
- **The process is stateless.** Replacing the binary or the container loses nothing. Two things
  outside the database must be kept safe, because the database alone cannot recover them:
  `SPHERICON_ENCRYPTION_KEY` and `SPHERICON_JWT_SECRET` (see [Backup and restore](./backup#secrets)).
- **Background work runs inside the same process.** Job workers and event consumers start with
  the server, so scaling out means running more replicas of the same image against the same
  database.

## Sizing

Treat these as a way of reasoning, not as benchmarks. Measure on your own traffic.

- **The database is the capacity limit.** Every replica opens its own connection pools: a
  `database/sql` pool for requests and event consumers, and a separate pool for the job queue.
  Add up the connections of all replicas and keep the total below the server's
  `max_connections`, leaving room for backups, `psql` sessions and monitoring. The formula and
  the defaults are in [Scaling and tuning](./scaling#connection-budget); a transaction-mode
  pooler in front of PostgreSQL is not recommended.
- **One replica is a sound starting point.** Add a second replica for availability, not for
  speed, until you see saturation. With more than one replica, run migrations as a separate
  step and leave `SPHERICON_DB_AUTO_MIGRATE` off (see [Upgrading](./upgrading)).
- **Memory and CPU are modest.** The process serves the API, the SPA and the tracker, and
  renders email. Sending volume is bounded by your provider (SMTP or Amazon SES) and its rate
  limits, not by the process.
- **Disk grows with events.** The `events` table and the domain-event outbox are the fastest
  growing data. Size the volume for your event rate and plan for growth; the backup size
  follows the same curve.
- **Prefer a managed PostgreSQL** (point-in-time recovery, automated failover, storage
  growth) over a database you patch yourself, if your environment offers one.

## Health checks

`/healthz` (liveness) and `/readyz` (readiness, pings the database) are documented in
[Self-hosting](../self-hosting#health-checks). Use `/readyz` to gate traffic during a rollout.

Metrics, proposed SLOs and example alerting rules are in [Monitoring](./monitoring); each alert
has a section in the [Runbook](./runbook).
