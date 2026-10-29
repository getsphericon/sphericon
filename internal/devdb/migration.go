package devdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"ariga.io/atlas/sql/sqltool"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/getsphericon/sphericon/ent/migrate"
)

// GenerateMigration writes the next migration, named name, from the ent schema into dir. ent's
// own versioned-migration engine (the Atlas library, no Atlas binary) diffs the schema against
// the migrations in dir using the scratch database diffURL and writes the SQL file (no atlas.sum:
// see sumlessDir).
func GenerateMigration(ctx context.Context, dir, diffURL, name string) error {
	// The diff engine opens the diff URL with the "postgres" driver name; pgx registers itself
	// as "pgx", so alias it (guarded: registering twice panics).
	if !slices.Contains(sql.Drivers(), "postgres") {
		sql.Register("postgres", stdlib.GetDefaultDriver())
	}
	if diffURL == "" {
		return errors.New("DIFF_DB_URL is not set (the diff database migrations are generated against)")
	}
	if err := requireDiffDatabase(ctx, diffURL); err != nil {
		return err
	}
	gooseDir, err := sqltool.NewGooseDir(dir)
	if err != nil {
		return fmt.Errorf("open migrations dir: %w", err)
	}
	if err := migrate.NamedDiff(ctx, diffURL, name,
		schema.WithDir(sumlessDir{gooseDir}),
		schema.WithMigrationMode(schema.ModeReplay),
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

// requireDiffDatabase fails with the task that creates the diff database when it is not there.
func requireDiffDatabase(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open the diff database: %w", err)
	}
	pingErr := db.PingContext(ctx)
	if err := db.Close(); err != nil {
		return fmt.Errorf("close the diff database: %w", err)
	}
	if pingErr != nil {
		return fmt.Errorf("the diff database (DIFF_DB_URL) is not reachable, run `mise run db:up` to create it: %w", pingErr)
	}
	return nil
}
