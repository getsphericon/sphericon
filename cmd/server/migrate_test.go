package main

import (
	"testing"
	"time"

	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// On an empty database applyMigrations creates the app schema and river's own
// tables, and is safe to run again.
func TestApplyMigrationsCreatesRiverSchemaOnEmptyDatabase(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL: testhelper.EmptyDBURL(t),
		DBPool:      config.DBPool{MaxOpenConns: 2, MaxIdleConns: 2, ConnMaxLifetime: time.Minute, PGXMaxConns: 2},
	}

	require.NoError(t, applyMigrations(cfg))
	require.NoError(t, applyMigrations(cfg))

	pool, err := appdb.NewPGXPool(t.Context(), cfg.DatabaseURL, cfg.DBPool)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	require.NoError(t, err)
	res, err := migrator.Validate(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, res.OK, "river migrations pending: %v", res.Messages)
}
