# Upgrading

An upgrade replaces the binary or image and applies any new database migrations. Migrations
are embedded in the binary and **forward-only**: there is no down migration, so the way back is
a restore, which is why every upgrade starts with a backup.

## Before you start

1. Read the notes for every version between yours and the target
   ([per-version notes](#per-version-notes)).
2. Take a database dump and confirm it finished. Keep `SPHERICON_ENCRYPTION_KEY` and `SPHERICON_JWT_SECRET`
   unchanged; see [Backup and restore](./backup).

   ```sh
   pg_dump --format=custom --file="sphericon-pre-upgrade.dump" \
     "postgres://backup_user@db.example.com:5432/sphericon?sslmode=require"
   ```

3. Note the version you are running (`./sphericon version`, or the image tag), so you can go back.

## Upgrade {#migrate}

**Single replica.** Replace the image or binary and restart. With `SPHERICON_DB_AUTO_MIGRATE=true` the
process applies pending migrations on startup.

**Multiple replicas.** Do not use `SPHERICON_DB_AUTO_MIGRATE`: replicas would race. Run the migration once,
then roll the servers.

1. Run the new version's migration step against the production database. It applies pending
   migrations and exits:

   ```sh
   docker run --rm \
     -e SPHERICON_ENV=production \
     -e DATABASE_URL="postgres://app@db.example.com:5432/sphericon?sslmode=require" \
     -e SPHERICON_JWT_SECRET="$SPHERICON_JWT_SECRET" \
     ghcr.io/getsphericon/sphericon:<new-version> migrate
   ```

   For the plain binary run `./sphericon migrate` with the same environment. Use it as a
   pre-deploy job or an init container. It applies the application migrations and river's
   own job-queue schema.

   On Kubernetes use the in-repo Helm chart (`charts/sphericon`): it runs `migrate` as a
   pre-install/pre-upgrade hook Job, so the Deployment only rolls after it succeeds. Create a
   Secret with `DATABASE_URL`, `SPHERICON_JWT_SECRET` and `SPHERICON_ENCRYPTION_KEY` first and pass its name as
   `existingSecret` (the chart refuses to render without it). Metrics stay off unless you set
   `metrics.enabled`; `metrics.serviceMonitor.enabled` adds a Prometheus Operator
   ServiceMonitor that scrapes the internal listener only.

2. Roll the replicas to the new version one at a time, waiting for `GET /readyz` to return
   `200` on each before moving on.

Pin the image to an explicit version tag in production instead of `latest`, so a restart
never upgrades you by accident.

## Roll back

Migrations only go forward, so a rollback means returning to the pre-upgrade state:

1. Stop every replica.
2. Restore the pre-upgrade dump into an empty database (see the
   [restore drill](./backup#restore-drill)) and point `DATABASE_URL` at it, or drop and
   recreate the database you are restoring over.
3. Start the **previous** image or binary.

Anything written after the dump is lost, and the points in
[After a restore](./backup#after-a-restore) apply. If the upgrade is only minutes old, weigh
whether fixing forward is cheaper than losing that window.

## Per-version notes {#per-version-notes}

sphericon is versioned with release-please from Conventional Commits. The authoritative list of
changes for each release is the
[GitHub releases page](https://github.com/getsphericon/sphericon/releases). The table below records
only what needs a self-hoster's attention beyond "run `migrate`": a new required setting, a
removed one, a slow migration, a changed default.

| Version | Self-hoster action                                                          |
| ------- | --------------------------------------------------------------------------- |
| 0.x     | Pre-release series. No special steps are recorded beyond running `migrate`. |

Read any row added here before you upgrade past that version.

## PostgreSQL major upgrades {#postgres}

A PostgreSQL major upgrade (for example 16 to 17) is a database administration task,
independent of sphericon releases. Do not combine it with an application upgrade: change one
thing at a time, so a failure has one suspect.

1. Check that the target major version is supported (see [Self-hosting](../self-hosting) for
   the supported range).
2. Take a fresh dump and rehearse the whole procedure on a scratch copy first.
3. Upgrade in place with `pg_upgrade`, or dump from the old server and restore into a new
   server on the new major version (`pg_dump` then `pg_restore`). Managed services offer
   their own major-version upgrade.
4. Stop sphericon during the switch, repoint `DATABASE_URL` if the server changed, start it, and
   check `GET /readyz`.
5. Run `ANALYZE` on the new cluster: planner statistics are not always carried over.

A physical backup or WAL archive from the old major version cannot be restored onto the new
one. Take a fresh base backup right after the upgrade, and keep the pre-upgrade `pg_dump` until
you are confident in the new cluster.
