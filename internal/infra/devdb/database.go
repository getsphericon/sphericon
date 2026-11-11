// Package devdb holds the database tasks of the development workflow that `cmd/db` runs: it
// creates and drops a checkout's database and writes the next goose migration from the ent schema.
package devdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
)

// Create creates the database dsn points at and says what it did: a database that is already
// there is left as it is.
func Create(ctx context.Context, dsn string) (string, error) {
	return admin(dsn, func(adminDB *sql.DB, dbName, ident string) (string, error) {
		// raw sql: CREATE DATABASE is a server command with an identifier, not a table operation.
		if _, err := adminDB.ExecContext(ctx, "CREATE DATABASE "+ident); err != nil {
			if pgErr := new(pgconn.PgError); errors.As(err, &pgErr) && pgErr.Code == pgerrcode.DuplicateDatabase {
				return fmt.Sprintf("database %s already exists, skipping", strconv.Quote(dbName)), nil
			}
			return "", fmt.Errorf("create database: %w", err)
		}
		return fmt.Sprintf("database %s created", strconv.Quote(dbName)), nil
	})
}

// Drop drops the database dsn points at, if it is there, and says what it did.
func Drop(ctx context.Context, dsn string) (string, error) {
	return admin(dsn, func(adminDB *sql.DB, dbName, ident string) (string, error) {
		// raw sql: DROP DATABASE is a server command with an identifier, not a table operation.
		if _, err := adminDB.ExecContext(ctx, "DROP DATABASE IF EXISTS "+ident); err != nil {
			return "", fmt.Errorf("drop database: %w", err)
		}
		return fmt.Sprintf("database %s dropped", strconv.Quote(dbName)), nil
	})
}

// admin runs fn against the server's `postgres` database, with the name of the database dsn
// points at and that name quoted as an identifier, for CREATE and DROP DATABASE, which take an
// identifier and not a bound parameter. dsn is any connection string pgx reads: a URL or
// keyword/value pairs.
func admin(dsn string, fn func(adminDB *sql.DB, dbName, ident string) (string, error)) (string, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return "", fmt.Errorf("parse SPHERICON_DATABASE_URL: %w", err)
	}
	dbName := cfg.Database
	cfg.Database = "postgres"

	adminDB := stdlib.OpenDB(*cfg)
	defer cleanup.Close(adminDB, "database")
	return fn(adminDB, dbName, pgx.Identifier{dbName}.Sanitize())
}
