package db

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
)

// RiverMigration is the goose migration that brings river's tables to riverVersion: river's own
// migrator (rivermigrate) runs it and records the version in river_migration itself, the way
// river's docs run it under another migration framework. It runs outside goose's transaction,
// because river applies each of its versions in a transaction of its own (some cannot share one).
// After a river upgrade whose schema moves, rivermigrate.Validate in run_test.go names the version
// that is due; add a RiverMigration for it to goMigrations. Its down step leaves river at the
// version before. River's versions 1 to 8 came in as the SQL files migrations/*_river_00N.sql,
// which stay.
func RiverMigration(version int64, riverVersion int) *goose.Migration {
	return goose.NewGoMigration(version,
		&goose.GoFunc{RunDB: migrateRiver(rivermigrate.DirectionUp, riverVersion)},
		&goose.GoFunc{RunDB: migrateRiver(rivermigrate.DirectionDown, riverVersion-1)})
}

func migrateRiver(direction rivermigrate.Direction, target int) func(context.Context, *sql.DB) error {
	return func(ctx context.Context, sqlDB *sql.DB) error {
		migrator, err := rivermigrate.New(riverdatabasesql.New(sqlDB), nil)
		if err != nil {
			return err
		}
		_, err = migrator.Migrate(ctx, direction, &rivermigrate.MigrateOpts{TargetVersion: target})
		return err
	}
}
