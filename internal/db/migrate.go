package db

import (
	"context"
	"database/sql"
	"io/fs"

	sphericon "github.com/getsphericon/sphericon"
	"github.com/pressly/goose/v3"
)

// Migrate applies the embedded migrations to sqlDB with goose: the app schema, river's
// tables and the domain-event outbox all come from them. It is the one path the server
// (`sphericon migrate`) and the test databases share, so a test runs against the schema
// production gets.
func Migrate(ctx context.Context, sqlDB *sql.DB) error {
	files, err := fs.Sub(sphericon.MigrationsFS, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, files)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
