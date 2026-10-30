package devdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"

	atlas "ariga.io/atlas/sql/schema"
	"ariga.io/atlas/sql/sqlclient"
	"ariga.io/atlas/sql/sqltool"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/getsphericon/sphericon/ent/migrate"
	"github.com/getsphericon/sphericon/internal/db"
)

// normalSchema is the scratch schema of the diff database the ent schema is normalized in: Atlas
// creates the ent tables there and reads them back the way Postgres stores them, and it does so
// only in a schema that holds no table.
const normalSchema = "ent_normal"

// GenerateMigration writes the next migration, named name, from the ent schema into dir. goose
// applies the migrations in dir to the scratch database diffURL, as it applies them everywhere
// else (each in a transaction of its own unless it says NO TRANSACTION), and ent's own
// versioned-migration engine (the Atlas library, no Atlas binary) diffs the schema against that
// database and writes the SQL file (no atlas.sum: see sumlessDir). ent's replay mode is not used:
// it runs the files outside any transaction, and what it leaves behind on a failure (a type, a
// function) breaks the next run.
func GenerateMigration(ctx context.Context, dir, diffURL, name string) (err error) {
	// The diff engine opens the diff URL with the "postgres" driver name; pgx registers itself
	// as "pgx", so alias it (guarded: registering twice panics).
	if !slices.Contains(sql.Drivers(), "postgres") {
		sql.Register("postgres", stdlib.GetDefaultDriver())
	}
	if diffURL == "" {
		return errors.New("DIFF_DB_URL is not set (the diff database migrations are generated against)")
	}
	if err := applyMigrations(ctx, dir, diffURL); err != nil {
		return err
	}
	gooseDir, err := sqltool.NewGooseDir(dir)
	if err != nil {
		return fmt.Errorf("open migrations dir: %w", err)
	}
	normal, err := sqlclient.Open(ctx, diffURL, sqlclient.OpenSchema(normalSchema))
	if err != nil {
		return fmt.Errorf("open the diff database: %w", err)
	}
	defer func() {
		if closeErr := normal.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close the diff database: %w", closeErr)
		}
	}()
	normalizer, ok := normal.Driver.(atlas.Normalizer)
	if !ok {
		return errors.New("the diff database's driver cannot normalize a schema")
	}
	// Inspect mode reads only the tables the ent schema names, so goose's version table, river's
	// tables and the outbox are not in the diff.
	if err := migrate.NamedDiff(ctx, diffURL, name,
		schema.WithDir(sumlessDir{gooseDir}),
		schema.WithMigrationMode(schema.ModeInspect),
		schema.WithDiffHook(normalized(ctx, normalizer)),
		// A removal is a destructive migration (ADR 0033): written here, guarded by hand.
		schema.WithDropColumn(true),
		schema.WithDropIndex(true),
		schema.WithDialect(dialect.Postgres),
		schema.WithFormatter(sqltool.GooseFormatter),
	); err != nil {
		return fmt.Errorf("generate migration: %w", err)
	}
	return nil
}

// normalized diffs the database against the ent schema as Postgres would store it. Postgres
// rewrites what it keeps (an index predicate's `IN (...)` comes back as `= ANY (ARRAY[...])`), so
// the schema as ent writes it differs from the same schema read back, and every such difference
// would land in the migration.
func normalized(ctx context.Context, n atlas.Normalizer) schema.DiffHook {
	return func(next schema.Differ) schema.Differ {
		return schema.DiffFunc(func(current, desired *atlas.Schema) ([]atlas.Change, error) {
			normal, err := n.NormalizeSchema(ctx, desired)
			if err != nil {
				return nil, fmt.Errorf("normalize the ent schema: %w", err)
			}
			return next.Diff(current, normal)
		})
	}
}

// applyMigrations empties the diff database, whatever an earlier run left in it, and applies the
// migrations in dir to it with goose. It fails with the task that creates the diff database when
// it is not there.
func applyMigrations(ctx context.Context, dir, dsn string) (err error) {
	diffDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open the diff database: %w", err)
	}
	defer func() {
		if closeErr := diffDB.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close the diff database: %w", closeErr)
		}
	}()
	if err := diffDB.PingContext(ctx); err != nil {
		return fmt.Errorf("the diff database (DIFF_DB_URL) is not reachable, run `mise run db:up` to create it: %w", err)
	}
	// raw sql: emptying a scratch database is a schema command, not a table operation.
	if _, err := diffDB.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+normalSchema+" CASCADE; DROP SCHEMA public CASCADE; "+
		"CREATE SCHEMA public; CREATE SCHEMA "+normalSchema); err != nil {
		return fmt.Errorf("empty the diff database: %w", err)
	}
	if _, err := db.MigrateFS(ctx, diffDB, os.DirFS(dir)); err != nil {
		return fmt.Errorf("apply the migrations to the diff database: %w", err)
	}
	return nil
}
