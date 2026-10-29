// Package devdb holds the database tasks of the development workflow that `cmd/db` runs: it
// creates and drops a checkout's database, writes the next goose migration from the ent schema and
// writes the migration for river's schema.
package devdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/getsphericon/sphericon/internal/cleanup"
)

// Create creates the database databaseURL points at and says what it did: a database that is
// already there is left as it is.
func Create(ctx context.Context, databaseURL string) (string, error) {
	return admin(ctx, databaseURL, func(adminDB *sql.DB, dbName, ident string) (string, error) {
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

// Drop drops the database databaseURL points at, if it is there, and says what it did.
func Drop(ctx context.Context, databaseURL string) (string, error) {
	return admin(ctx, databaseURL, func(adminDB *sql.DB, dbName, ident string) (string, error) {
		// raw sql: DROP DATABASE is a server command with an identifier, not a table operation.
		if _, err := adminDB.ExecContext(ctx, "DROP DATABASE IF EXISTS "+ident); err != nil {
			return "", fmt.Errorf("drop database: %w", err)
		}
		return fmt.Sprintf("database %s dropped", strconv.Quote(dbName)), nil
	})
}

// admin runs fn against the server's `postgres` database, with the name of the database
// databaseURL points at and that name quoted as an identifier.
func admin(ctx context.Context, databaseURL string, fn func(adminDB *sql.DB, dbName, ident string) (string, error)) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	u.Path = "/postgres"

	adminDB, err := sql.Open("pgx", u.String())
	if err != nil {
		return "", fmt.Errorf("open admin db: %w", err)
	}
	defer cleanup.Close(adminDB, "database")
	ident, err := quoteIdent(ctx, adminDB, dbName)
	if err != nil {
		return "", err
	}
	return fn(adminDB, dbName, ident)
}

// quoteIdent has the server quote a database name, so no escaping of our own stands between a
// name from DATABASE_URL and the CREATE or DROP DATABASE that takes it: those commands accept an
// identifier and not a bound parameter.
func quoteIdent(ctx context.Context, adminDB *sql.DB, name string) (string, error) {
	var quoted string
	// raw sql: quote_ident is the server's own identifier quoting; ent has no such function.
	if err := adminDB.QueryRowContext(ctx, "SELECT quote_ident($1)", name).Scan(&quoted); err != nil {
		return "", fmt.Errorf("quote database name: %w", err)
	}
	return quoted, nil
}
