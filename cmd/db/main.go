package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/url"
	"os"
	"slices"
	"strings"

	"ariga.io/atlas/sql/sqltool"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/migrate"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: db <create|drop|river-migration|generate <name>>")
	}

	envName := os.Getenv("APP_ENV")
	if envName == "" {
		envName = "development"
	}

	cfg, err := config.Load(envName)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// river-migration writes the goose migration for river's schema (see river.go).
	if os.Args[1] == "river-migration" {
		if err := generateRiverMigration("migrations"); err != nil {
			log.Fatalf("river migration: %v", err)
		}
		return
	}

	// generate writes the next Atlas migration from the ent schema. The community
	// Atlas binary cannot read ent:// itself, so ent's own versioned-migration engine
	// (the Atlas library) diffs the schema against migrations/ using the scratch
	// ATLAS_DEV_URL database and writes the SQL file plus atlas.sum.
	if os.Args[1] == "generate" {
		generateMigration(os.Args[2:])
		return
	}

	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("parse DATABASE_URL: %v", err)
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	u.Path = "/postgres"

	adminDB, err := sql.Open("pgx", u.String())
	if err != nil {
		log.Fatalf("open admin db: %v", err)
	}
	defer func() { _ = adminDB.Close() }()

	switch os.Args[1] {
	case "create":
		_, err = adminDB.ExecContext(context.Background(), `CREATE DATABASE "`+dbName+`"`)
		if err != nil {
			if pgErr := new(pgconn.PgError); errors.As(err, &pgErr) && pgErr.Code == "42P04" {
				log.Printf("database %q already exists, skipping", dbName)
				return
			}
			log.Fatalf("create database: %v", err)
		}
		log.Printf("database %q created", dbName)
	case "drop":
		_, err = adminDB.ExecContext(context.Background(), `DROP DATABASE IF EXISTS "`+dbName+`"`)
		if err != nil {
			log.Fatalf("drop database: %v", err)
		}
		log.Printf("database %q dropped", dbName)
	default:
		log.Fatalf("unknown command %q, expected create or drop", os.Args[1])
	}
}

func generateMigration(args []string) {
	// Atlas opens the dev URL with the "postgres" driver name; pgx registers itself
	// as "pgx", so alias it (guarded: registering twice panics).
	if !slices.Contains(sql.Drivers(), "postgres") {
		sql.Register("postgres", stdlib.GetDefaultDriver())
	}
	if len(args) != 1 || args[0] == "" {
		log.Fatal("usage: db generate <migration-name>")
	}
	devURL := os.Getenv("ATLAS_DEV_URL")
	if devURL == "" {
		log.Fatal("ATLAS_DEV_URL is not set (the scratch database Atlas diffs against)")
	}
	dir, err := sqltool.NewGooseDir("migrations")
	if err != nil {
		log.Fatalf("open migrations dir: %v", err)
	}
	if err := migrate.NamedDiff(context.Background(), devURL, args[0],
		schema.WithDir(dir),
		schema.WithMigrationMode(schema.ModeReplay),
		schema.WithDialect(dialect.Postgres),
		schema.WithFormatter(sqltool.GooseFormatter),
	); err != nil {
		log.Fatalf("generate migration: %v", err)
	}
}
