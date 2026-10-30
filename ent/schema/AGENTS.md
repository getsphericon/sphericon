# Database and migrations


- ORM is **ent**; schemas live in `ent/schema/*.go`, generated code in `ent/`.
- Migrations: **ent diffs, goose applies, Squawk lints, CI guards immutability.** `mise run db:generate <desc>`
  (`cmd/db`, ent's diff engine as a library, no CLI and no checksum file) diffs the ent schema against
  `migrations/` in the scratch `<PGDATABASE>_diff` database (`DIFF_DB_URL`) and writes a goose `.sql` file;
  `mise run db:migrate` (`sphericon migrate`) applies it with goose, the one runner of dev, tests and
  production; `cmd/upgradecheck` runs Squawk over the added files; and `cmd/upgradecheck immutable`
  fails a PR that rewrites a migration already on `main` unless its body has `History-Collapse:`
  (ADR 0047). A developer whose dev database predates this runs `mise run db:reset` once.
- **`migrations/` is the whole schema**: the app tables (diffed from ent), river's tables and the
  domain-event outbox (watermill-sql's two tables, a hand-written migration: `ent/schema/outbox_*.go`
  model them as skipped entities, ent's diff leaves them alone). Nothing creates schema at boot: the
  server runs the same goose migrations (`internal/db.Migrate`, `sphericon migrate`), and the
  tests' template databases are built by that same function, so tests run against the schema
  production gets. After an `ent/schema` edit the tests fail until `mise run db:generate` has written
  its migration.
- river's tables are one migration per river version (`*_river_00N.sql`, river applies each version
  in a transaction of its own), written by `mise run db:river-migration` from the river version in
  `go.mod`; after a river upgrade run it again and `rivermigrate.Validate` in
  `internal/db/run_test.go` tells you when it is due. Do not edit those files by hand.
- **Migrations are immutable.** `cmd/upgradecheck immutable` (in `check:upgrade`) fails a PR that
  modifies or deletes a goose `.sql` or Go migration already on `main`, naming each file; only a PR
  whose body has a line `History-Collapse: <reason>` may rewrite history.
- **Migration rules (ADR 0033).** Between two Majors a migration only adds (a table, a nullable
  column, an index built `CONCURRENTLY`, a new payload field); a removal or a rename waits for a
  Major (before 1.0 every `0.MINOR` is one). Heavy data work is a river job that runs after the
  Upgrade, never a migration: a migration is fast DDL. A long DDL is built concurrently without a
  transaction (`CREATE INDEX CONCURRENTLY` under `-- +goose NO TRANSACTION`). A destructive
  migration starts with a check that the data it strands has moved (a `DO` block that raises), so
  an Upgrade that skipped a release fails instead of losing data. CI (`mise run check:upgrade`,
  `cmd/upgradecheck`) lints the migrations a PR adds, skipping the generated river ones, and fails
  a destructive finding unless the PR title is breaking (`!`), and a blocking one (an index without
  `CONCURRENTLY`, a constraint without `NOT VALID`, `SET NOT NULL`) under any title unless the
  statement has `-- squawk-ignore <rule>` on the line before it; a breaking PR title or commit needs
  an `Upgrade:` line inside its `BREAKING CHANGE:` footer, in a commit message or the PR body,
  saying what a Self-hoster does (`BREAKING CHANGE: <what broke>` then a line `Upgrade: <what to do>`;
  release-please renders only the text of that footer into CHANGELOG.md, never a footer of its own,
  and `sphericon upgrade-notes` reads the `Upgrade:` lines from its BREAKING CHANGES section; the
  repository squashes with the PR title and the commit messages).
- The schema is plain tables: no database functions, triggers or views (`CODING_STANDARDS.md`).
- `mise run db:reset` / `db:reset-test` to rebuild local DBs. A database made before the history
  collapse (`20261024175209_init.sql` holds the whole app schema, the outbox and river's tables stay
  their own migrations) needs a `db:reset`. The project is greenfield, so the next collapse is the
  same one `History-Collapse:` line and a `db:reset` for everyone.
- Test DB is separate (`SPHERICON_ENV=test`); tests get it from the migrations, see docs/agents/dev-environment.md.


Schema consistency: `mise run check:schema` runs [schemalens](https://github.com/mokevnin/schemalens)
(pinned in `.mise.toml`, its ent adapter fetched into the user cache by `mise/scripts/schemalens.sh`)
against the migrated dev database and compares it with the ent schema. Each finding carries a `fix`
for the ent schema: apply it, `mise run db:generate`, re-run. An exclusion needs a `reason` and the
user's say-so (`schemalens.yml`). CI runs it in the `test` job, not in `check`, because it needs the
migrated database. An enum field gets its CHECK from `TableWithEnumChecks` in its schema's
`Annotations()`; a new migration that adds a constraint or an index follows the Squawk rules above
(`NOT VALID` then `VALIDATE`, `CONCURRENTLY`, both under `-- +goose NO TRANSACTION`).
