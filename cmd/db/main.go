package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"slices"
	"strings"

	"ariga.io/atlas/sql/sqltool"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/migrate"
)

func main() {
	if err := newRoot().Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func newRoot() *cli.Command {
	return &cli.Command{
		Name:  "db",
		Usage: "database tasks of the dev workflow",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unknown command %q (see `db --help`)", cmd.Args().First())
			}
			return errors.New("a command is required (see `db --help`)")
		},
		Commands: []*cli.Command{
			{
				Name:  "create",
				Usage: "create this checkout's database",
				Action: func(ctx context.Context, _ *cli.Command) error {
					return admin(ctx, createDatabase)
				},
			},
			{
				Name:  "drop",
				Usage: "drop this checkout's database",
				Action: func(ctx context.Context, _ *cli.Command) error {
					return admin(ctx, dropDatabase)
				},
			},
			{
				Name:  "river-migration",
				Usage: "write the goose migration for river's schema",
				Action: func(context.Context, *cli.Command) error {
					return generateRiverMigration("migrations")
				},
			},
			{
				Name:      "generate",
				Usage:     "write the next Atlas migration from the ent schema",
				ArgsUsage: "<migration-name>",
				Arguments: []cli.Argument{&cli.StringArg{Name: "name", Required: true}},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return generateMigration(ctx, cmd.StringArg("name"))
				},
			},
		},
	}
}

// admin runs fn against the server's `postgres` database, with the name of the database
// DATABASE_URL points at.
func admin(ctx context.Context, fn func(ctx context.Context, adminDB *sql.DB, dbName string) error) error {
	envName := os.Getenv("SPHERICON_ENV")
	if envName == "" {
		envName = "development"
	}
	cfg, err := config.Load(envName)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	u.Path = "/postgres"

	adminDB, err := sql.Open("pgx", u.String())
	if err != nil {
		return fmt.Errorf("open admin db: %w", err)
	}
	defer func() { _ = adminDB.Close() }()
	return fn(ctx, adminDB, dbName)
}

func createDatabase(ctx context.Context, adminDB *sql.DB, dbName string) error {
	if _, err := adminDB.ExecContext(ctx, `CREATE DATABASE "`+dbName+`"`); err != nil {
		if pgErr := new(pgconn.PgError); errors.As(err, &pgErr) && pgErr.Code == "42P04" {
			log.Printf("database %q already exists, skipping", dbName)
			return nil
		}
		return fmt.Errorf("create database: %w", err)
	}
	log.Printf("database %q created", dbName)
	return nil
}

func dropDatabase(ctx context.Context, adminDB *sql.DB, dbName string) error {
	if _, err := adminDB.ExecContext(ctx, `DROP DATABASE IF EXISTS "`+dbName+`"`); err != nil {
		return fmt.Errorf("drop database: %w", err)
	}
	log.Printf("database %q dropped", dbName)
	return nil
}

// generateMigration writes the next Atlas migration from the ent schema. The community
// Atlas binary cannot read ent:// itself, so ent's own versioned-migration engine
// (the Atlas library) diffs the schema against migrations/ using the scratch
// ATLAS_DEV_URL database and writes the SQL file plus atlas.sum.
func generateMigration(ctx context.Context, name string) error {
	// Atlas opens the dev URL with the "postgres" driver name; pgx registers itself
	// as "pgx", so alias it (guarded: registering twice panics).
	if !slices.Contains(sql.Drivers(), "postgres") {
		sql.Register("postgres", stdlib.GetDefaultDriver())
	}
	devURL := os.Getenv("ATLAS_DEV_URL")
	if devURL == "" {
		return errors.New("ATLAS_DEV_URL is not set (the scratch database Atlas diffs against)")
	}
	dir, err := sqltool.NewGooseDir("migrations")
	if err != nil {
		return fmt.Errorf("open migrations dir: %w", err)
	}
	if err := migrate.NamedDiff(ctx, devURL, name,
		schema.WithDir(dir),
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
