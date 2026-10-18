# Backup and restore

Everything sphericon knows lives in PostgreSQL, so backing up the database backs up the instance.
Two secrets live **outside** the database and need their own backup. This page covers what to
back up, how, how to rehearse a restore, and what the instance does once it is restored.

## What to back up

| Item                       | Where it lives                  | Why it matters                                                                                        |
| -------------------------- | ------------------------------- | ----------------------------------------------------------------------------------------------------- |
| The PostgreSQL database    | Your database server            | Contacts, events, Outbound messages, automations, the job queue and the event outbox.                 |
| `SPHERICON_ENCRYPTION_KEY` | Your secret store / environment | Decrypts the provider credentials stored in the database. Without it a restored database cannot send. |
| `SPHERICON_JWT_SECRET`     | Your secret store / environment | Signs session tokens. Losing it signs every user out; it does not lose data.                          |

### Secrets {#secrets}

Back up `SPHERICON_ENCRYPTION_KEY` and `SPHERICON_JWT_SECRET` **separately from the database dump**, in your secret
manager or password vault, with its own access control. Do not store them next to the dump: a
dump that travels with its key defeats the encryption of the stored provider credentials.

- A restored database paired with a **different** `SPHERICON_ENCRYPTION_KEY` cannot decrypt the stored
  SMTP or SES credentials. You would have to re-enter every provider credential.
- A **different** `SPHERICON_JWT_SECRET` only invalidates existing sessions; users sign in again.

## Minimum: scheduled `pg_dump`

A logical dump is the simplest backup and is enough for a small instance that can tolerate
losing the time since the last dump.

```sh
pg_dump --format=custom --file="sphericon-$(date -u +%Y%m%dT%H%M%SZ).dump" \
  "postgres://backup_user@db.example.com:5432/sphericon?sslmode=require"
```

Supply the password through `PGPASSWORD` or a `~/.pgpass` file, not on the command line.

- Run it on a schedule (cron, a Kubernetes `CronJob`) at least daily.
- Ship the file off the database host, to storage in another failure domain, and keep several
  generations.
- Use a role that can read the whole database, and nothing else.
- `pg_dump` takes a consistent snapshot without blocking the running instance.

## Production: point-in-time recovery

For production, add continuous WAL archiving so you can restore to any moment, not just the
last dump. Your options, in order of effort:

- A **managed PostgreSQL** service with automated backups and point-in-time recovery.
- A backup tool such as pgBackRest or Barman that archives WAL and takes base backups.

Keep taking periodic `pg_dump` files as well: they are portable across PostgreSQL major
versions, which a physical backup is not (see [Upgrading](./upgrading#postgres)).

## Restore drill {#restore-drill}

A backup you have never restored is a hope, not a backup. Rehearse into a scratch database on
a schedule (for example monthly) and after any change to the backup setup.

1. Create an empty database on a scratch PostgreSQL server (same major version as production,
   or newer) and restore the dump:

   ```sh
   createdb --host=scratch.example.com sphericon_restore
   pg_restore --no-owner \
     --dbname="postgres://admin@scratch.example.com:5432/sphericon_restore?sslmode=require" \
     sphericon-20260101T000000Z.dump
   ```

2. Start the **same version** of sphericon that wrote the dump against the scratch database, with
   the backed-up `SPHERICON_ENCRYPTION_KEY` and `SPHERICON_JWT_SECRET`. Point it at a throwaway SMTP sink (for
   example Mailpit), not at your real provider, so a drill cannot send real mail. Leave
   `SPHERICON_DB_AUTO_MIGRATE` off.

   ```sh
   docker run --rm -p 3000:3000 \
     -e SPHERICON_ENV=production \
     -e DATABASE_URL="postgres://app@scratch.example.com:5432/sphericon_restore?sslmode=require" \
     -e SPHERICON_JWT_SECRET="$SPHERICON_JWT_SECRET" \
     -e SPHERICON_ENCRYPTION_KEY="$SPHERICON_ENCRYPTION_KEY" \
     -e SPHERICON_APP_URL="https://restore-drill.example.com" \
     -e SPHERICON_SMTP_HOST=mailpit.internal -e SPHERICON_SMTP_PORT=1025 \
     ghcr.io/getsphericon/sphericon:<version>
   ```

3. Check that `GET /readyz` returns `200`, sign in, and open a workspace. Confirm the contact
   and event counts match what you expect.

4. Record how long it took. That is your real recovery time.

If you restore into production instead of a scratch server, stop every replica first, restore,
then start one replica and verify it before scaling back out.

## After a restore

Restoring puts the database back at the backup point. The instance carries on from there, and
three behaviours follow from the architecture:

- **Background jobs restart from the restored queue.** The job queue (river) is stored in the
  database, so jobs that were pending at the backup point are pending again, and jobs that
  finished after it are no longer recorded as finished. Workers pick up what the restored
  queue says is due.
- **Domain events are redelivered at least once.** The event outbox is also in the database.
  Consumers resume from their stored position, so anything published after that position is
  delivered again. An Event is stored once per `source_id`, so redelivery does not duplicate
  it.
- **Mail sent after the backup point can be sent again.** Each email sphericon sends is recorded
  as an Outbound message before the provider is called, and the idempotency key that prevents
  duplicate sends lives in that same database row
  ([ADR 0015](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0015-outbound-send-single-chokepoint.md)). A message sent after the backup
  point has no row in the restored database, so its send is not recognized as already done
  and a still-pending job or automation step can send it again. Delivery to the provider is
  at-least-once by design.

Practical consequences:

- After restoring, consider pausing broadcasts and automations until you have checked what
  was in flight at the time of the failure, especially if the gap between the backup and the
  failure was long.
- Prefer point-in-time recovery to a moment just before the failure over restoring the last
  nightly dump: the smaller the gap, the fewer messages can repeat.
- Contacts who unsubscribed, bounced or complained after the backup point are not in the
  restored database. If your provider or your own records show such events, replay them; an
  address that opted out must not be mailed again.
