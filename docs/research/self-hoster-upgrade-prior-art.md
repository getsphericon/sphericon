# Self-hoster upgrade prior art: migrations, rollback guards, self-update, breaking-change notes

Researched 2026-10-18 from primary sources only: shallow clones of each project's default branch (commit SHAs
below, permalinks use them), single files fetched from raw.githubusercontent.com, and each project's own docs
site. No blog posts, Stack Overflow or aggregators. Anything not confirmed in a primary source is marked
**unverified**. Limits: Consul and Nomad were not examined (Vault's docs stand in for HashiCorp); Hasura's and
Mattermost's startup locking were only partly read; Cal.com and Umami were read at the container-start script
level only. "Not found" means a targeted search of the named file(s) found nothing, not that it cannot exist
elsewhere in the repo.

| Repo                        | Commit                                     |
| --------------------------- | ------------------------------------------ |
| go-gitea/gitea              | `d0c35bc`                                  |
| mattermost/mattermost       | `f5227e66`                                 |
| grafana/grafana (sparse)    | `3920186`                                  |
| knadh/listmonk              | `524039e`                                  |
| goauthentik/authentik       | `0bf3eb2`                                  |
| metabase/metabase (sparse)  | `ba35fd1af0c98b8ed39254d5f11b184223b3916d` |
| temporalio/temporal         | `d934169`                                  |
| hasura/graphql-engine       | `037916c`                                  |
| hashicorp/vault             | `7649ad4`                                  |
| rudderlabs/rudder-server    | `bba1c21`                                  |
| rudderlabs/rudderstack-helm | `81496b1`                                  |
| pressly/goose               | `6d6e4a4`                                  |
| golang-migrate/migrate      | `504568a`                                  |
| ariga/atlas                 | `1879644`                                  |
| riverqueue/river            | `f043d98`                                  |

Permalink form: `https://github.com/<org>/<repo>/blob/<commit>/<path>#L<n>`. Gitea's docs, Directus, n8n, Plausible,
PocketBase, Caddy, PostHog, Sentry were read from `main`/`master` raw files without pinning a SHA.

## 1. Comparison table

Columns: migration trigger | multi-replica safety | newer-schema guard | single-command upgrade |
breaking-change communication | backup guidance. "Guard" = the binary refuses (or only warns) when the DB is newer
than it knows.

| Project                | Migration trigger                                                         | Multi-replica safety                                                          | Newer-schema guard                                                                                        | Single-command upgrade                                                | Breaking-change comms                                                        | Backup guidance                                               |
| ---------------------- | ------------------------------------------------------------------------- | ----------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------- |
| **RudderStack**        | On startup, inside jobsdb setup ([setup.go:28](#rudderstack))             | Yes: `pg_advisory_xact_lock` around schema migrate                            | **Opposite**: force-resets a lower version in the migrations table (default on); assumes compat changes   | `helm upgrade` (no hook Job; app migrates itself)                     | release-please changelog; no BREAKING section in `CHANGELOG.md` (0 hits)     | Not found in the docs read (**unverified**)                   |
| **Gitea**              | On startup (`Migrate` via `InitDBEngine`); `gitea migrate` command exists | No lock found in `modelmigration/migrations.go`                               | **Yes, fatal**: "is for a newer Gitea"                                                                    | `contrib/upgrade.sh` (download, sha256, gpg, dump, stop, swap, start) | Changelog on blog, "check breaking changes"; admin-panel deprecation banners | Documented as mandatory; `upgrade.sh` runs `gitea dump`       |
| **GitLab**             | Omnibus/Helm run migrations; post-deployment migrations split out         | Documented zero-downtime procedure (`SKIP_POST_DEPLOYMENT_MIGRATIONS`)        | Not found (rollback = restore backup)                                                                     | `helm upgrade` documented; skipping stops forbidden                   | Per-major "upgrade notes" + required upgrade stops                           | Required: "Create a rollback plan and backup"                 |
| **Mattermost**         | On startup (morph); `mattermost db migrate` / `db downgrade` exist        | Yes: `morph.WithLock("mm-lock-key")`                                          | Not found as startup refusal (**unverified**); docs: downgrade needs `db downgrade` with the newer binary | Documented manual steps (no single command)                           | "Important upgrade notes" table filterable by from/to version                | Required before upgrade and downgrade                         |
| **Grafana**            | On startup                                                                | Yes: advisory lock, `migration_locking = true` default                        | Not found (**unverified**)                                                                                | Package manager / image tag                                           | One upgrade-guide page per version                                           | Told to restore backup after a v13.0.0 migration bug          |
| **Metabase**           | On startup (Liquibase); `migrate down`                                    | Liquibase lock table; waits, then fails with release-locks hint               | **Yes, fatal**: "ERROR: Downgrade detected."                                                              | Swap JAR/image; auto-migrates                                         | Release notes + upgrade guide                                                | "Back up your application database" step 1                    |
| **Listmonk**           | Explicit: `listmonk --upgrade`; non-final builds auto-run                 | No lock found                                                                 | Opposite direction only: refuses to start when schema is _older_                                          | `./listmonk --upgrade` then run                                       | Release notes (not examined)                                                 | Interactive prompt "Take a backup" + docs warning             |
| **Authentik**          | On startup (`lifecycle/migrate.py`)                                       | Yes: `pg_advisory_lock`                                                       | **Warn only** on downgrade; **hard stop** on skipped major (minor) versions                               | `docker compose pull && up`                                           | Upgrade page, from/to link embedded in the error message                     | Required step; restore backup on "migration inconsistency"    |
| **Temporal**           | Explicit schema tool step, then start server                              | Not applicable (tool run once)                                                | **Yes, fatal if DB is older**; deliberately allows newer (rollback-tolerant)                              | Multi-step (tool, then server)                                        | Schema changes "called out directly in the release notes"                    | Not stated in the page excerpt read                           |
| **Hasura**             | On startup (catalog); `graphql-engine downgrade` command                  | **Unverified**                                                                | Not found; ships down-scripts + app-to-catalog version map                                                | Change image tag                                                      | Changelog                                                                    | Not examined                                                  |
| **Vault**              | No SQL; storage may change on upgrade                                     | n/a                                                                           | "no backward-compatibility guarantees for the data store"                                                 | Swap binary, restart, unseal                                          | Per-major "important changes" + deprecation notices                          | Snapshot before; restore into an isolated test instance first |
| **Sentry self-hosted** | Inside `./install.sh` (`web upgrade`)                                     | n/a (single installer)                                                        | Not found                                                                                                 | `git checkout <tag> && ./install.sh`                                  | "Hard stops" list                                                            | Separate Backup & Restore page                                |
| **PostHog (hobby)**    | `upgrade-hobby` script + `bin/migrate-check`                              | n/a                                                                           | Not found                                                                                                 | `curl ... upgrade-hobby \| bash` (prompts, says downtime)             | Not examined                                                                 | Warns to back up first                                        |
| **Plausible CE**       | Compose command chains `db createdb && db migrate && run`                 | No lock found (**unverified**)                                                | Not found                                                                                                 | `git pull <tag> && docker compose up -d`                              | Per-release notes; changelog                                                 | Not examined                                                  |
| **Directus**           | Container entrypoint runs `bootstrap` (idempotent migrate) each start     | No lock in `run.ts`                                                           | Warns only if migrations pending                                                                          | Image tag change                                                      | Not examined                                                                 | Not examined                                                  |
| **n8n**                | On startup                                                                | Yes: Postgres advisory lock                                                   | Not found at startup; `db:revert` fails with explicit text if code for last migration is missing          | Image tag change                                                      | Not examined                                                                 | Not examined                                                  |
| **Umami / Cal.com**    | `prisma migrate deploy` in the container start path                       | Prisma has a schema advisory lock (env `PRISMA_SCHEMA_DISABLE_ADVISORY_LOCK`) | Not found                                                                                                 | Image tag change                                                      | Not examined                                                                 | Not examined                                                  |
| **goose** (library)    | Caller's choice                                                           | Opt-in: `WithSessionLocker` (`pg_try_advisory_lock`); off by default          | **None**: DB versions unknown to the source are ignored                                                   | n/a                                                                   | n/a                                                                          | n/a                                                           |
| **golang-migrate**     | Caller's choice                                                           | `pg_advisory_lock` on by default in Postgres driver                           | Fails with "no migration found for version N" when it needs to read it                                    | n/a                                                                   | n/a                                                                          | n/a                                                           |
| **Atlas**              | `migrate apply`                                                           | `Driver.Lock` around apply, `--lock-timeout`                                  | Not found (only errors on a _partially_ applied unknown last revision)                                    | n/a                                                                   | n/a                                                                          | n/a                                                           |
| **river** (migrator)   | Caller's choice                                                           | No lock found in `rivermigrate/river_migrate.go`                              | **Yes**: "migration N not found in migrator bundle"                                                       | n/a                                                                   | n/a                                                                          | n/a                                                           |

Evidence links per row are in sections 2 to 6 and the Sources list.

## 2. Q1: when migrations run, and are replicas safe?

Three camps.

- **Auto-migrate on startup under a lock** (Mattermost, Grafana, Authentik, n8n, RudderStack, Prisma users such as
  Umami and Cal.com, Metabase via Liquibase's lock table).
- **Auto-migrate on startup, no lock found** (Gitea, Directus). Gitea's `Migrate` is a plain loop over
  `getPendingMigrations` with no lock
  ([migrations.go:512](https://github.com/go-gitea/gitea/blob/d0c35bc/modelmigration/migrations.go#L512)).
  Directus's `latest()` applies pending migrations and inserts a row each, no lock
  ([run.ts](https://raw.githubusercontent.com/directus/directus/main/api/src/database/migrations/run.ts)).
- **Explicit step** (Listmonk `--upgrade`, Temporal schema tools, Sentry's `install.sh`, GitLab's Helm migrations
  Job). Even these usually add a startup check that the step was done (section 3).

Lock mechanics seen:

- Authentik: `"""lock an advisory lock to prevent multiple instances from migrating at once"""`, `SELECT pg_advisory_lock(%s)`
  ([lifecycle/migrate.py:58-64](https://github.com/goauthentik/authentik/blob/0bf3eb2/lifecycle/migrate.py#L58)).
- Grafana: `migration_locking = true` by default, with `locking_attempt_timeout_sec`; Postgres/MySQL only
  ([conf/defaults.ini:293](https://github.com/grafana/grafana/blob/3920186/conf/defaults.ini#L293),
  [migrator.go:225](https://github.com/grafana/grafana/blob/3920186/pkg/services/sqlstore/migrator/migrator.go#L225)).
- Mattermost: `morph.WithLock("mm-lock-key")`
  ([sqlstore/migrate.go:128](https://github.com/mattermost/mattermost/blob/f5227e66/server/channels/store/sqlstore/migrate.go#L128)).
- n8n: `migrateWithAdvisoryLock`, doc comment "so concurrently starting instances migrate one at a time";
  SQLite skipped as single-instance
  ([db-connection.ts](https://raw.githubusercontent.com/n8n-io/n8n/master/packages/@n8n/db/src/connection/db-connection.ts)).
- Metabase: Liquibase lock; on contention it retries then errors "Database has migration lock; cannot run
  migrations." with a `migrate release-locks` hint
  ([liquibase.clj:230-244](https://github.com/metabase/metabase/blob/ba35fd1/src/metabase/app_db/liquibase.clj#L230)).
- goose: `WithSessionLocker` "enables locking"; "If WithSessionLocker is not called, locking is disabled"
  ([provider_options.go:70](https://github.com/pressly/goose/blob/6d6e4a4/provider_options.go#L70)); the Postgres
  locker polls `pg_try_advisory_lock` with retry
  ([lock/postgres.go:109](https://github.com/pressly/goose/blob/6d6e4a4/lock/postgres.go#L109)), default lock ID
  `4097083626` ([session_locker_options.go:13](https://github.com/pressly/goose/blob/6d6e4a4/lock/session_locker_options.go#L13)).
- golang-migrate: `pg_advisory_lock`, "This will wait indefinitely until the lock can be acquired"
  ([postgres.go:241](https://github.com/golang-migrate/migrate/blob/504568a/database/postgres/postgres.go#L241)).
- Atlas: `client.Driver.Lock(ctx, applyLockValue, flags.lockTimeout)`
  ([cmdapi/migrate.go:1758](https://github.com/ariga/atlas/blob/1879644/cmd/atlas/internal/cmdapi/migrate.go#L1758)).
- river's migrator: no advisory lock found in `rivermigrate/river_migrate.go` (each migration runs in a tx); river
  is irrelevant for sphericon's own tables now (they are goose migrations), noted for completeness.

**Expand/contract and rolling updates are communicated, but only by the mature products.**

- Mattermost's upgrade-notes table states per migration: "Previous versions of Mattermost will simply ignore the new
  column, ensuring seamless compatibility across a rolling upgrade. The migrations are fully backwards-compatible and
  no database downtime is expected", and warns about `ACCESS EXCLUSIVE` locks and `CREATE INDEX CONCURRENTLY` for
  big tables
  ([important-upgrade-notes](https://docs.mattermost.com/administration-guide/upgrade/important-upgrade-notes.html)).
- GitLab splits post-deployment migrations out of the deploy and tells you to run them last on a zero-downtime
  upgrade
  ([zero_downtime](https://docs.gitlab.com/update/zero_downtime/)).
- Temporal: "we only make backwards compatible schema changes"
  ([schema/version.go:15](https://github.com/temporalio/temporal/blob/d934169/common/persistence/schema/version.go#L15)).
- Grafana tells you not to run multi-instance with migrations disabled: its unified-storage `migration_locking`
  comment warns that disabling is "only safe with no concurrent writers (single instance, no HA, no rolling
  upgrades)" ([defaults.ini:2758](https://github.com/grafana/grafana/blob/3920186/conf/defaults.ini#L2758)).

<a id="rudderstack"></a>
**RudderStack (closest analog).** `setupDatabaseTables` runs the versioned SQL migrator at jobsdb start
([jobsdb/setup.go:28](https://github.com/rudderlabs/rudder-server/blob/bba1c21/jobsdb/setup.go#L28)), inside
`WithTx` and a distributed lock: "only one migration should run at a time and block all other processes from adding
or removing tables"
([jobsdb_lifecycle.go:109-111](https://github.com/rudderlabs/rudder-server/blob/bba1c21/jobsdb/jobsdb_lifecycle.go#L109)),
implemented as `SELECT pg_advisory_xact_lock(%d)`
([jobsdb_tx.go:198](https://github.com/rudderlabs/rudder-server/blob/bba1c21/jobsdb/jobsdb_tx.go#L198)). So: Go,
Postgres, migrate-on-start under an advisory lock, no separate migration Job. The Helm chart repo has no migration
hook: the only `helm.sh/hook` is a `test-success` hook
(`grep -rn helm.sh/hook` in [rudderstack-helm](https://github.com/rudderlabs/rudderstack-helm/tree/81496b1)).

## 3. Q2: newer-schema guard and rollback

**Refuse to start, with a specific message:**

- Gitea (fatal, tells you to use the right release and not to edit the version by hand):

  > Your database (migration version: %d) is for a newer Gitea, you can not use the newer database for this old Gitea
  > release (%d). Gitea will exit to keep your database safe and unchanged. Please use the correct Gitea release, do
  > not change the migration version manually (incorrect manual operation may lose data).

  [migrations.go:546](https://github.com/go-gitea/gitea/blob/d0c35bc/modelmigration/migrations.go#L546). It also
  refuses the other direction for read-only admin tools: "current database version %d is not equal to the expected
  version %d. Please run "gitea ... migrate"" ([L497](https://github.com/go-gitea/gitea/blob/d0c35bc/modelmigration/migrations.go#L497)).

- Metabase (the best-written one: states the fact, lists the offending changesets, gives the exact command, links docs):

  > ERROR: Downgrade detected. Your metabase instance appears to have been downgraded without a corresponding
  > database downgrade. You must run `java ... -jar metabase.jar migrate down` from version {0}. Once your database
  > has been downgraded, try running the application again.

  Preceded by a warning `Database has migrations from v%d but this binary only knows up to v%d:` plus each changeset
  ([app_db/setup.clj:213-236](https://github.com/metabase/metabase/blob/ba35fd1/src/metabase/app_db/setup.clj#L213)).

- river's migrator: `migration %d not found in migrator bundle`
  ([river_migrate.go:210](https://github.com/riverqueue/river/blob/f043d98/rivermigrate/river_migrate.go#L210)).
- n8n `db:revert`: "The last migration that was executed is "X", but I could not find that migration's code in the
  currently installed version of n8n ... This usually means that you downgraded n8n before running `n8n db:revert`.
  Please upgrade n8n again and run `n8n db:revert` and then downgrade again."
  ([revert.ts](https://raw.githubusercontent.com/n8n-io/n8n/master/packages/cli/src/commands/db/revert.ts)).

**Warn only:** Authentik: `LOGGER.warning("Unsupported downgrade detected", ...)`, with the comment "Downgrades are
not supported, but we don't stop them (for now)"
([migrate.py:99-103](https://github.com/goauthentik/authentik/blob/0bf3eb2/lifecycle/migrate.py#L99)).

**Deliberately tolerate newer (rollback by design, because migrations are backward compatible):**

- Temporal's startup check fails only when the DB is _older_ than the binary: "if after a schema upgrade there is a
  code rollback ... This check [allows] such rollbacks since we only make backwards compatible schema changes";
  error `version mismatch for keyspace/database: %q. Expected version: %s cannot be greater than Actual version: %s`
  ([version.go:9-28](https://github.com/temporalio/temporal/blob/d934169/common/persistence/schema/version.go#L9),
  called at boot from [fx.go:190](https://github.com/temporalio/temporal/blob/d934169/temporal/fx.go#L190)).
- RudderStack: the migrator **forces the version in the migrations table down** to the latest file on a rollback,
  with `SQLMigrator.forceSetLowerVersion` defaulting to `true`: "force set version in DB to latestSourceVersion to
  handle cases where we are reverting back to old version; this assumes applied changes on database are also
  compatible with older versions"
  ([sql-migrator/migrator.go:252-272](https://github.com/rudderlabs/rudder-server/blob/bba1c21/services/sql-migrator/migrator.go#L252)).
  It logs "does not undo the additional migrations done in version specified in DB".
  This is the opposite of a guard and only safe under a strict expand-only discipline.

**Not found (so no guard, and the library will not give you one):**

- goose ignores DB versions it has no file for: `UpVersions` only looks for files missing below the DB max
  ([gooseutil/resolve.go:19-62](https://github.com/pressly/goose/blob/6d6e4a4/internal/gooseutil/resolve.go#L19));
  there is no `ErrMissingVersion`-style error for "DB newer than source". The sphericon guard must be hand-written
  (compare `Provider.GetDBVersion` with the highest embedded version).
- Atlas: if the last revision is fully applied and not in the directory, `Pending` returns nothing and no error;
  `MissingMigrationError` fires only for a _partially_ applied unknown last revision
  ([migrate.go:720-760](https://github.com/ariga/atlas/blob/1879644/sql/migrate/migrate.go#L720)).
- golang-migrate: reading an unknown version yields `no migration found for version %d`
  ([migrate.go:807](https://github.com/golang-migrate/migrate/blob/504568a/migrate.go#L807)), only on paths that read
  it.

**What the docs tell users to do to roll back.** Almost all say: restore the pre-upgrade backup.

- Gitea: "Since you can not run an old Gitea with an upgraded database, a backup should always be made before a
  database upgrade"; patch releases (`a.b.x`) are interchangeable, minor bumps are not
  ([upgrade-from-gitea](https://docs.gitea.com/installation/upgrade-from-gitea)).
- Metabase: "we recommend reverting to a backup of your application database to roll back an upgrade ... only using
  the `migrate down` command if you need to keep changes made after your upgrade"
  ([upgrading-metabase](https://www.metabase.com/docs/latest/installation-and-operation/upgrading-metabase)).
- Mattermost ships down migrations and a command: use the newer binary and `mattermost db downgrade 128,127,126`;
  "We don't recommend downgrading more than one major version back"
  ([downgrading](https://docs.mattermost.com/administration-guide/upgrade/downgrading-mattermost-server.html)).
- Hasura ships `<new>_to_<old>.sql` down scripts and an app-version to catalog-version map
  ([Migrate.hs:5-14](https://github.com/hasura/graphql-engine/blob/037916c/server/src-lib/Hasura/Server/Migrate.hs#L5)),
  and a `graphql-engine downgrade` command
  ([docs](https://hasura.io/docs/2.0/deployment/downgrading/)).
- Vault: snapshot first; "Vault does not make backward-compatibility guarantees for the Vault data store"
  ([upgrading](https://developer.hashicorp.com/vault/docs/upgrading)).

## 4. Q3: single-command upgrades

**Self-update commands in single-binary products** (few exist; most delegate to the package manager or image tag):

| Tool       | What it does                                                                                                                                                              | Verification                                                   | Restart / migrate                                                                   |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| PocketBase | `./pocketbase update`: fetch latest GitHub release, find asset by OS/arch suffix, rename current exe to `.old`, move new in, revert on failure; `--backup` zips `pb_data` | No checksum/signature code found in `ghupdate.go` (HTTPS only) | Does not restart; migrations run on next start (not re-verified)                    |
| Caddy      | `caddy upgrade`: replace the binary, keep a backup, remove it on success                                                                                                  | No signature/hash code found in `cmd/packagesfuncs.go`         | "upgrade successful; please restart any running Caddy instances" (does not restart) |
| Gitea      | No self-update. `contrib/upgrade.sh`: downloads from dl.gitea.com, **sha256sum, optional gpg**, runs `gitea dump`, `systemctl stop`, swaps, `systemctl start`             | sha256 + gpg                                                   | Stops/starts the systemd unit; migration happens on next start                      |
| PostHog    | `upgrade-hobby` piped from curl: prompts "This will cause a few minutes of downtime", compose pull/up                                                                     | None                                                           | Restarts the stack                                                                  |
| Sentry     | `git checkout <tag>; ./install.sh; docker compose up --wait`: installer runs checks, builds, `web upgrade`                                                                | `check-latest-commit.sh` refuses a stale checkout              | Migrates inside the installer; compose up is a second command                       |
| Listmonk   | Manual: overwrite the binary, `./listmonk --upgrade`, run again. Prompts for backup confirmation                                                                          | n/a                                                            | Refuses to start until `--upgrade` ran                                              |

Sources: [ghupdate.go](https://raw.githubusercontent.com/pocketbase/pocketbase/master/plugins/ghupdate/ghupdate.go),
[caddy packagesfuncs.go:175-219](https://raw.githubusercontent.com/caddyserver/caddy/master/cmd/packagesfuncs.go),
[gitea upgrade.sh](https://github.com/go-gitea/gitea/blob/d0c35bc/contrib/upgrade.sh),
[posthog upgrade-hobby](https://raw.githubusercontent.com/PostHog/posthog/master/bin/upgrade-hobby),
[sentry releases](https://develop.sentry.dev/self-hosted/releases/),
[listmonk upgrade docs](https://listmonk.app/docs/upgrade/).

**Helm.** The Helm docs define `pre-upgrade`: "Executes on an upgrade request after templates are rendered, but
before any resources are updated"
([charts_hooks](https://helm.sh/docs/topics/charts_hooks/)), with `hook-delete-policy`. GitLab's chart ships a
dedicated migrations Job, backoff-limited
([_jobspec.yaml:23](https://gitlab.com/gitlab-org/charts/gitlab/-/blob/master/charts/gitlab/charts/migrations/templates/_jobspec.yaml));
whether it uses a hook annotation was **not** found in that file. Its upgrade doc is a single `helm upgrade` with
pre-steps (backup, one minor at a time)
([upgrade.md](https://gitlab.com/gitlab-org/charts/gitlab/-/raw/master/doc/installation/upgrade.md)).
RudderStack's chart README: "To update configuration or version of the images used, change the configuration and
run: `helm upgrade my-release ./ --set rudderWorkspaceToken=...`" and nothing else
([README.md:36-42](https://github.com/rudderlabs/rudderstack-helm/blob/81496b1/README.md#L36)); the docs repeat it
([kubernetes setup](https://www.rudderstack.com/docs/get-started/rudderstack-open-source/data-plane-setup/kubernetes/)).

**Docker Compose.** Plausible: `git pull origin <tag>` then `docker compose up -d`, with
`command: sh -c "/entrypoint.sh db createdb && /entrypoint.sh db migrate && /entrypoint.sh run"` in the compose
file ([compose.yml:39](https://raw.githubusercontent.com/plausible/community-edition/master/compose.yml);
[Upgrade wiki](https://raw.githubusercontent.com/wiki/plausible/community-edition/Upgrade.md)). Directus' entrypoint
runs `directus bootstrap` every start ("idempotent DB install/migrate + ensure admin")
([docker-entrypoint.cjs](https://raw.githubusercontent.com/directus/directus/main/docker-entrypoint.cjs)).
Cal.com's `start.sh` runs `npx prisma migrate deploy` then `yarn start`
([start.sh](https://raw.githubusercontent.com/calcom/cal.com/main/scripts/start.sh)). All three: the new image
migrates itself, so the upgrade is `pull` + `up`.

## 5. Q4: communicating breaking changes and required operator actions

- **Per-version table/page in the docs**: Mattermost "Important upgrade notes" (filter From/To version; each row
  says whether downtime or locks are expected); GitLab "upgrade notes" per major and "required upgrade stops"
  ([upgrade_paths](https://docs.gitlab.com/update/upgrade_paths/)); Grafana one page per release
  (`docs/sources/upgrade-guide/upgrade-v13.0/index.md` documents automatic unified-storage migration "on startup",
  tracked in `unifiedstorage_migration_log`); Vault "important changes for each major version"; Sentry "Hard
  Stops" list; Authentik upgrade page with `?from=..&to=..` links.
- **In the error text itself**: Authentik's skip error embeds a docs URL with the from/to versions
  ([migrate.py:115](https://github.com/goauthentik/authentik/blob/0bf3eb2/lifecycle/migrate.py#L115)); Metabase's
  downgrade error ends with the docs anchor; Gitea/Listmonk errors give the command to run.
- **In-product warnings**: Gitea shows deprecated-config warnings at the top of the admin panel "for at least one
  release cycle", and "Gitea may refuse to start in the following version" if unresolved (upgrade docs, above);
  Temporal shows a notification in the UI when a newer server exists
  ([upgrade-server](https://docs.temporal.io/self-hosted-guide/upgrade-server)); Listmonk has an update checker
  (`App.checkUpdates`, [updates.go:36](https://github.com/knadh/listmonk/blob/524039e/cmd/updates.go#L36)); Mattermost
  logs a warning and auto-rewrites a removed setting at startup (upgrade notes, atmos/camo example).
- **Release-note conventions**: Plausible: "Each new release of Plausible contains specific instructions on how to
  upgrade from the previous version" and a SemVer-ish pinning scheme (`v2`, `v2.1`, `v2.1.4`) in the wiki. Temporal:
  schema changes "will be called out directly in the release notes". release-please recognizes `feat!:` /
  `fix!:` and a `BREAKING CHANGE:` footer
  ([customizing.md:52,87](https://github.com/googleapis/release-please/blob/main/docs/customizing.md)).
  RudderStack uses release-please too, yet its `CHANGELOG.md` has **0** `BREAKING` entries (grep) and a search of
  all GitHub release bodies found "BREAKING CHANGES:" only in v1.20.0 and v0.1.5, so the convention exists but is
  barely used; **no tooling enforces the "required operator action" part** in any project I read (RudderStack's
  `semantic-pr.yaml` validates only the PR title format).
- **Enforcement by tooling**: none found for operator-action notes. What does exist is enforcement in the binary
  (Authentik's major-skip refusal, Gitea's too-old/too-new fatals, Listmonk's pending-upgrade fatal).

## 6. Q5: backup and version skipping

- **Documented, not automated, is the norm**: Gitea, GitLab, Mattermost, Metabase, Listmonk, Authentik, Vault,
  Sentry, PostHog all say "back up first". Metabase makes it step 1 of 4.
- **Automated or prompted by tooling**: Gitea's `upgrade.sh` runs `gitea dump` (`backupopts` var);
  PocketBase `update --backup` zips `pb_data`; Listmonk's `--upgrade` prompts "** IMPORTANT: Take a backup of the
  database before upgrading." and aborts without a "y" unless `--yes`
  ([upgrade.go:54-66](https://github.com/knadh/listmonk/blob/524039e/cmd/upgrade.go#L54)). No project found that runs
  a `pg_dump` by itself before migrating (none seen; absence is **unverified** beyond the files read).
- **A real incident that justifies it**: Grafana's v13.0 upgrade guide says a migration bug in v13.0.0 could lose
  dashboards with Git Sync, "restore from the database backup you took before upgrading"
  ([upgrade-v13.0/index.md:25-35](https://github.com/grafana/grafana/blob/3920186/docs/sources/upgrade-guide/upgrade-v13.0/index.md)).
- **Version skipping**:
  - Hard rule in code: Authentik compares the DB's last version with the previous version family and raises
    `Major version skips are not allowed` unless `migrations.dangerously_allow_multiple_major_version_upgrades`
    ([migrate.py:76-117](https://github.com/goauthentik/authentik/blob/0bf3eb2/lifecycle/migrate.py#L76)).
  - Hard rule in docs: GitLab (required stops, "Upgrade one minor release at a time ... If you skip releases,
    database modifications might be run in the wrong sequence"), Sentry (hard stops list), Temporal ("one minor
    version at a time"; "no guarantee that there is compatibility between any two non-consecutive versions"), Gitea
    (`minDBVersion`: "upgrade to a lower version first (suggested v1.6.4)",
    [L539](https://github.com/go-gitea/gitea/blob/d0c35bc/modelmigration/migrations.go#L539)).
  - None: Grafana ("You can upgrade to any of the supported releases from any supported release", upgrade-guide
    index), RudderStack (no rule found, **unverified**; monthly releases, `1-alpine` tag auto-updates per
    [releases.md](https://github.com/rudderlabs/rudder-server/blob/bba1c21/releases.md)).

## 7. Patterns

**Recurring (best practice):**

1. Migrations ship in the binary and run before serving traffic, under a Postgres advisory lock (Authentik, Grafana,
   Mattermost, n8n, RudderStack, Prisma, golang-migrate). Locking is cheap; its absence (Gitea, Directus) is the
   usual source of "do not start two replicas at once" folklore.
2. The binary checks schema version at boot and refuses on mismatch, with the remediation in the message (Gitea,
   Metabase, Listmonk, Temporal).
3. Backward-compatible (expand-only) migrations when rolling or HA is supported, stated per migration in the notes
   (Mattermost, GitLab, Temporal, RudderStack).
4. Restore-from-backup is the official rollback; `down` migrations are a secondary tool (Gitea, Metabase, Mattermost).
5. One upgrade page per version or an upgrade-notes table with from/to filter, plus an in-error docs link.
6. Self-update never restarts the service itself; it swaps the binary atomically with a `.old` rollback copy
   (PocketBase, Caddy). Gitea's script stops and starts systemd explicitly.

**Rare:**

- Checksum plus signature verification in an updater (only Gitea's script; PocketBase and Caddy rely on TLS).
- A guard that tolerates newer schemas by design (Temporal, RudderStack). It is only safe with a strict expand-only
  policy and a version where the guard distinguishes "compatible newer" from "incompatible newer", which none of them
  do (Temporal compares one version number).
- Tooling-enforced breaking-change notes; automatic pre-migration backup of an external Postgres.
- Version-skip enforcement in code (only Authentik).

## 8. Recommendation for sphericon

Labels: **[2+]** supported by two or more projects' primary-source evidence; **[judgement]** a design call.

**(a) Migrate on start, or keep an explicit step?** Do both, with one rule: _every start runs the same code path_.

- Default `AUTO_MIGRATE=true` for the plain binary and Docker/Compose; the new binary migrates under a goose session
  locker (`lock.NewPostgresSessionLocker`, own lock ID) before listening. Replicas serialize, losers wait then see
  "nothing to do". **[2+]** Authentik, Grafana, Mattermost, n8n, RudderStack, Prisma; goose has the locker built in
  (opt-in, so it must be switched on explicitly; today's reason for the default being off goes away).
- Keep `sphericon migrate` as the explicit step and use it in Helm and ECS pre-deploy tasks (below). With
  `AUTO_MIGRATE=false` the process still runs the guard and **refuses to start if the schema is older than the
  binary**, naming `sphericon migrate`. **[2+]** Listmonk, Gitea (`EnsureUpToDate`), Temporal.
- A session-level advisory lock needs a direct (non-transaction-pooled) connection. **[judgement]** (Grafana's
  and golang-migrate's code use session locks; RudderStack uses `pg_advisory_xact_lock` inside a tx, which survives
  pooling and could be used instead.) Pick one deliberately.
- Implied rule: **expand/contract migrations**, because ECS and Kubernetes rolling updates run old and new code
  against the migrated schema. Write it into the contributor docs: add nullable columns / new tables / `CREATE INDEX
  CONCURRENTLY`; drop or rename only one release later (GitLab post-deployment migrations, Mattermost notes).
  **[2+]** Mattermost, GitLab, Temporal, RudderStack. A contract step (drop column) ships in a later minor and is
  called out in the changelog.

**(b) Newer-schema guard.** goose will not do it for you (`UpVersions` ignores unknown DB versions), so add one, run
before `Up`, after taking the lock:

- Compare `provider.GetDBVersion(ctx)` with the highest embedded migration version. If DB > binary: exit non-zero
  before binding any port. **[2+]** Gitea, Metabase, river's migrator (all fatal).
- Proposed text (modelled on Metabase and Gitea, one fact, one cause, one action, one link):

  ```
  sphericon: database schema is newer than this binary.
    database schema version: 20260912103000 (applied by sphericon v0.4.2)
    this binary (v0.3.1) knows up to: 20260801090000
  This usually means the binary was rolled back after an upgrade. Starting would risk data loss, so sphericon exits.
  To fix: run sphericon v0.4.2 or newer, or restore the database backup taken before the upgrade.
  Rolling back: https://<docs>/upgrading#rolling-back
  ```

- Record the app version next to each applied migration (a `sphericon_version_history` table, as Authentik does in
  `authentik_version_history`) so the message can name the version that wrote the schema. **[judgement]**
  (Authentik is the only one doing it.)
- Escape hatch `ALLOW_NEWER_SCHEMA=true` (log a warning, like Authentik's downgrade warning) for operators doing a
  deliberate one-patch rollback. **[judgement]** Temporal and RudderStack tolerate newer schemas by design, but only
  because they promise expand-only changes; sphericon's migrations are forward-only without such a promise yet, so
  the default must be refuse. Revisit once the expand/contract rule has held for several releases.
- Same startup check when the DB is _older_ and `AUTO_MIGRATE=false` (above). Both messages in one function, tested
  through `testhelper` as scenarios (leave a newer migration row via goose, assert exit and message).

**(c) `sphericon upgrade`.** Scope it as a binary swap plus the migration, nothing else.

Do:

1. Resolve target (latest GitHub release or `--version`); refuse a downgrade unless `--allow-downgrade` and refuse a
   skip over a documented required-stop version. **[2+]** GitLab, Sentry, Authentik, Temporal (all enforce stops);
   sphericon has none yet, so the check is empty until the first stop exists.
2. Download the matching `tar.gz` and `checksums.txt` for GOOS/GOARCH and **verify sha256**. **[2+ for swap, 1 for
   verification]** PocketBase/Caddy swap without verification; only Gitea's `upgrade.sh` verifies (sha256 + gpg).
   Cosign-verify `checksums.txt` too if GoReleaser signs it (**unverified** whether sphericon's pipeline does).
3. Print pending migrations from the new binary (`sphericon migrate --plan`), require `--yes` in a non-TTY.
   **[2+]** Listmonk's prompt, PostHog's prompt.
4. Atomic replace: write next to the target, `rename` old to `.old`, `rename` new in, roll back on failure.
   **[2+]** PocketBase, Caddy.
5. Run the new binary's `migrate` (explicit, lock held) so failures surface in the terminal, not in a restart loop.
   **[judgement]** (the others migrate on next start; here the user typed one command and wants one result).

Do not:

- Do not restart the service by default. PocketBase and Caddy do not. Offer `--restart` that runs `systemctl restart
  sphericon` only when it detects a systemd unit (Gitea's script is the precedent for stop/start).
  **[judgement]**
- Do not `pg_dump` for the user (Postgres is external, can be huge, needs credentials and a matching `pg_dump`).
  Print the backup warning and the exact command instead; offer `--backup-cmd '<shell>'` to run an operator-chosen
  command and abort on failure. **[judgement]** (no project found auto-dumps an external Postgres; PocketBase backs
  up its own embedded data dir and Gitea's script uses `gitea dump`).
- Do not touch Docker/Helm installs; detect a container (`/.dockerenv`, KUBERNETES_SERVICE_HOST) and print the
  right command instead.

**(d) Helm and Docker Compose.**

- **Helm**: `helm upgrade sphericon <chart> --version X` is the one command. Keep the `pre-upgrade` (and
  `pre-install`) hook Job running `sphericon migrate` with `hook-delete-policy: before-hook-creation,hook-succeeded`,
  and set `AUTO_MIGRATE=false` on the Deployment so N replicas never race and a failed migration fails the release
  before any pod is replaced. **[2+]** Helm docs define the hook semantics; GitLab's chart uses a dedicated
  migrations Job. Counter-evidence: RudderStack's chart has no hook and lets the app migrate under its lock, which
  would also work for sphericon. The Deployment still runs the guard, so a rolled-back image refuses to start with
  the clear error instead of crash-looping silently. A `helm rollback` after a migration needs the same care as any
  rollback: the older image will be refused by the guard unless the release was expand-only (document this).
- **Compose**: `docker compose pull && docker compose up -d`, with `AUTO_MIGRATE=true` in the shipped compose file,
  and a `migrate` one-shot service is unnecessary. **[2+]** Plausible (chains `db migrate && run` in the compose
  command), Directus (`bootstrap` on every start), Cal.com. Pin the image tag (`vMAJOR.MINOR.PATCH`) in the compose
  file and document major/minor pinning as Plausible does.
- **ECS (own SaaS)**: run `sphericon migrate` as a one-off task before updating the service (the same shape as
  the Helm hook), or rely on the lock with `AUTO_MIGRATE=true`. **[judgement]**

**(e) Surfacing breaking changes with release-please.**

- Conventional Commits `feat!:` / `BREAKING CHANGE:` footer become a "BREAKING CHANGES" section in the changelog
  (release-please docs, above). Require, via the existing commitlint, that a `!` commit has a footer line starting
  `Upgrade:` (what the operator must do). **[judgement]**; RudderStack shows the failure mode when nothing requires it
  (0 BREAKING entries in `CHANGELOG.md`).
- One `docs/upgrading.md` with a per-version table (from, to, required action, migration is expand-only or not, any
  required stop), filterable like Mattermost's. **[2+]** Mattermost, GitLab, Grafana, Vault, Sentry. Add a CI check
  that a release PR touching migrations has a row (mirror Temporal's "schema changes are called out in the release
  notes"). **[judgement]**
- Put the docs link in every guard error and in `sphericon upgrade`'s output. **[2+]** Authentik, Metabase.
- Pre-1.0: release-please can bump minor for breaking changes; treat every minor as a possible required stop until
  1.0. **[judgement]**
- Optional later: an in-product notice when `sphericon version --check` finds a newer release (Listmonk
  `checkUpdates`, Temporal's UI notification). **[2+]** but not a launch need.

**(f) Backup guidance.** Document, do not automate, as everyone does. **[2+]** In `docs/self-hosting.md`: (1) back up
Postgres before every upgrade, including patch releases (Gitea: "even if the upgrade is between patch versions"),
(2) restore is the supported rollback, (3) `sphericon upgrade` and the Helm values print the reminder and honor
`--backup-cmd`, (4) restore into a scratch DB and start the old binary against it to test the path once (Vault's
isolated test-instance advice), (5) note river and outbox tables are inside the same DB, so one `pg_dump` covers
the queue and the outbox (do not restore only part of the DB). The Grafana v13.0.0 incident is the argument for
making the reminder loud.

## Sources

Code (commit-pinned, see table above): the `blob/<commit>/...` links inline in sections 2 to 6.

Docs and raw files (read 2026-10-18):

- Gitea: <https://docs.gitea.com/installation/upgrade-from-gitea>
- GitLab: <https://docs.gitlab.com/update/upgrade_paths/>, <https://docs.gitlab.com/update/zero_downtime/>,
  <https://docs.gitlab.com/update/plan_your_upgrade/>, <https://docs.gitlab.com/update/background_migrations/>,
  <https://gitlab.com/gitlab-org/charts/gitlab/-/raw/master/doc/installation/upgrade.md>
- Mattermost: <https://docs.mattermost.com/administration-guide/upgrade/upgrading-mattermost-server.html>,
  <https://docs.mattermost.com/administration-guide/upgrade/downgrading-mattermost-server.html>,
  <https://docs.mattermost.com/administration-guide/upgrade/important-upgrade-notes.html>
- Grafana: `docs/sources/upgrade-guide/_index.md`, `docs/sources/upgrade-guide/upgrade-v13.0/index.md` in grafana/grafana
- Metabase: <https://www.metabase.com/docs/latest/installation-and-operation/upgrading-metabase>
- Listmonk: <https://listmonk.app/docs/upgrade/>
- Temporal: <https://docs.temporal.io/self-hosted-guide/upgrade-server>
- Hasura: <https://hasura.io/docs/2.0/deployment/downgrading/>
- Vault: <https://developer.hashicorp.com/vault/docs/upgrading>
- Sentry: <https://develop.sentry.dev/self-hosted/releases/>, <https://develop.sentry.dev/self-hosted/backup/>,
  `install.sh` and `install/*.sh` in getsentry/self-hosted
- PostHog: <https://posthog.com/docs/self-host/deploy/upgrade-hobby>, `bin/upgrade-hobby`, `bin/docker-server`
- Plausible: community-edition `compose.yml`, wiki `Upgrade.md`; analytics `CHANGELOG.md`
- Directus: `api/src/database/migrations/run.ts`, `api/src/app.ts`, `docker-entrypoint.cjs`
- n8n: `packages/@n8n/db/src/connection/db-connection.ts`, `packages/cli/src/commands/db/revert.ts`
- Umami `package.json`, `scripts/check-db.js`; Cal.com `scripts/start.sh`; Prisma CLI reference (env var
  `PRISMA_SCHEMA_DISABLE_ADVISORY_LOCK`) and migrate deploy docs
- RudderStack: rudder-server `jobsdb/setup.go`, `jobsdb/jobsdb_lifecycle.go`, `jobsdb/jobsdb_tx.go`,
  `services/sql-migrator/migrator.go`, `CHANGELOG.md`, `releases.md`, `.github/workflows/semantic-pr.yaml`;
  rudderstack-helm `README.md`; <https://www.rudderstack.com/docs/get-started/rudderstack-open-source/data-plane-setup/kubernetes/>
- PocketBase `plugins/ghupdate/ghupdate.go`; Caddy `cmd/packagesfuncs.go`
- Helm hooks: <https://helm.sh/docs/topics/charts_hooks/>
- release-please: <https://github.com/googleapis/release-please/blob/main/docs/customizing.md>

Could not verify from primary sources: Consul/Nomad (not examined); locking in Gitea beyond the migration function,
Hasura startup, Plausible and Directus; Mattermost/Grafana newer-schema refusal; whether GitLab's migrations Job is a
Helm hook; any RudderStack version-skip rule or backup guidance; whether sphericon's GoReleaser config signs
`checksums.txt`.
