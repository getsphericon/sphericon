---
status: accepted
---

# An Upgrade only adds within a Major, and never stops a running Instance

A self-hosted Instance is upgraded by whoever runs it, often several releases late, while customer
sites keep sending events to it. So an Upgrade has to work from any release, and neither it nor a
failed one may take a running Instance down: the tracker would lose the events of that window. The
rules below make a jump safe without asking the Self-hoster to step through every release. The
research behind them is `docs/research/self-hoster-upgrade-prior-art.md`.

## Decisions

- **Releases between two Majors only add.** A new table, a nullable column, an index built
  `CONCURRENTLY`, a new field of a job or event payload. Removing or renaming any of them (a
  column, a table, a job kind, a payload field) is allowed only in a **Major**. An Upgrade may skip
  any releases inside a Major; across Majors it goes through the last release of each Major it
  crosses. Until 1.0 each `0.MINOR` is a Major.
- **A destructive migration comes with a breaking commit** (`!` or `BREAKING CHANGE:`), so release-please
  cuts the Major itself, and CI runs Squawk over the new migrations and fails a
  destructive finding (the `DestructiveRules` table in `cmd/upgradecheck`: a dropped or renamed
  column or table, a changed column type, a NOT NULL column without a default) whose commit is not
  breaking; Squawk's other rules are advice and never fail. The migration starts with a check that the data it
  strands is already moved (a `DO` block that raises), so an Upgrade that skipped a release fails
  instead of losing data. Job and event payloads follow the same rule as columns.
- **Heavy data work is a river job after the Upgrade, never a migration.** A migration is fast
  DDL; an unavoidable long one uses `CONCURRENTLY` and `-- +goose NO TRANSACTION`.
- **Migrations are explicit by default.** `SPHERICON_DB_AUTO_MIGRATE` stays off; `sphericon migrate`
  is the one primitive, run by `sphericon upgrade`, the Helm hook and the ECS task. Switched on, a
  start migrates under a Postgres session advisory lock, so replicas take turns, for at most
  `SPHERICON_DB_MIGRATE_TIMEOUT` (default ten minutes, `0` for no limit). `sphericon migrate`
  itself has no time limit.
- **The database records its Major and a floor**, one row (`schema_state`) holding the schema's
  Major (a counter, 1 for the first, plus one per destructive migration) and the **floor**: the
  oldest release that still runs on it, which is the last release of the previous Major. Only a
  destructive migration changes it, writing both. A process refuses to start when its release is
  older than the floor (a rollback past a removal; the message names the release the database
  needs) and when the database is two or more Majors behind it (a skipped Major; it names the
  last release of the database's Major, to install first). When the database is one Major behind it
  migrates if `SPHERICON_DB_AUTO_MIGRATE` is on, otherwise refuses and names `sphericon migrate`.
  Inside a Major it does the same for migrations the database has not run: a binary's queries name
  the columns its own migrations add, so without auto-migrate it refuses and names `sphericon
  migrate` rather than start and answer with a bare 500. In development (not `SPHERICON_ENV=production`) a
  running process also rechecks on its requests (cached for seconds) and answers `503` with a
  problem naming the command while migrations are missing, which covers a database reset under
  the dev stack; the probes pass. Production has only the start guard: a check of its own could
  stop a running Instance, which this ADR forbids.
  It does not refuse merely because the database's Major is higher: the last release of a Major
  is exactly the floor of the next, so it starts on the next Major's schema and an interrupted
  Upgrade leaves the Instance running. A release inside a Major runs on the schema of any other
  release of it once its own migrations have run, and an older release runs on a newer schema. Every refusal names the version, the commit and a
  link to the upgrade guide. A build with no release number (`dev`) is never older than a floor.
- **The new version migrates before it serves.** An Upgrade runs the migration with the new binary
  while the old one keeps running, and only then swaps and restarts. An interruption leaves the old
  binary on a schema it can use. A refusal or a failed migration blocks the Upgrade, never a
  running Instance.
- **Events survive the restart.** What is left is the gap of a restart on a single host, covered by
  the tracker and `/collect`: unsent events queue in the browser and retry, `/collect` takes a
  client event id and treats a repeat as a no-op, and answers `503` with `Retry-After` when the
  database is unavailable. That work has its own ticket and is a dependency of the promise.

## Considered Options

- **Upgrade one release at a time.** Rejected: a chain of Upgrades, each a restart, for whoever is
  five releases behind.
- **A "required stop" declared per release** (GitLab, Sentry). Rejected: a decision on every
  release, easy to forget, and the Major already says where stops are.
- **Self-contained migrations that move the data themselves.** Rejected: long migrations stop a
  running Instance, which is the one thing an Upgrade must not do.
- **Migrate on every start by default.** Rejected: a heavy migration under an orchestrator's
  liveness probe is the failure this ADR exists to avoid; it stays an explicit choice.

## Consequences

A destructive change waits for the next Major and is split into an add in a release and a removal
in the next Major, so a field is deprecated for a whole Major. The state row and the guard ship
before 1.0, where every `0.MINOR` is a Major, because the next release already needs them. The
destructive migration and its release must agree: it writes the new Major and the floor to the
row, and adds the same pair to `Majors` in `internal/db`, which `sphericon upgrade` also walks.
