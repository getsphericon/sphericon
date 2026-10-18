package main

import (
	"bytes"
	"database/sql"
	"testing"
	"time"

	"github.com/pressly/goose/v3/lock"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// On an empty database applyMigrations creates the app schema, river's tables and the
// domain-event outbox, and is safe to run again.
func TestApplyMigrationsCreatesRiverAndOutboxSchemaOnEmptyDatabase(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL: testhelper.EmptyDBURL(t),
		DBPool:      config.DBPool{MaxOpenConns: 2, MaxIdleConns: 2, ConnMaxLifetime: time.Minute, PGXMaxConns: 2},
	}

	_, err := applyMigrations(t.Context(), cfg)
	require.NoError(t, err)
	_, err = applyMigrations(t.Context(), cfg)
	require.NoError(t, err)

	pool, err := appdb.NewPGXPool(t.Context(), cfg.DatabaseURL, cfg.DBPool)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	require.NoError(t, err)
	res, err := migrator.Validate(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, res.OK, "river migrations pending: %v", res.Messages)

	// The outbox the producer writes and the consumers read exists, empty.
	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	client := appdb.NewEntClient(sqlDB)
	messages, err := client.OutboxMessage.Query().Count(t.Context())
	require.NoError(t, err)
	require.Zero(t, messages)
	offsets, err := client.OutboxOffset.Query().Count(t.Context())
	require.NoError(t, err)
	require.Zero(t, offsets)
}

func emptyConfig(t *testing.T, timeout time.Duration) *config.Config {
	t.Helper()
	return &config.Config{
		DatabaseURL:    testhelper.EmptyDBURL(t),
		MigrateTimeout: timeout,
		DBPool:         config.DBPool{MaxOpenConns: 2, MaxIdleConns: 2, ConnMaxLifetime: time.Minute, PGXMaxConns: 2},
	}
}

// `migrate --status` prints the schema's state and the pending migrations and changes nothing.
func TestMigrationStatusListsPendingAndChangesNothing(t *testing.T) {
	cfg := emptyConfig(t, 0)

	var before bytes.Buffer
	require.NoError(t, printMigrationStatus(t.Context(), cfg, &before))
	require.Contains(t, before.String(), "schema:  empty")
	require.Contains(t, before.String(), "pending: ")
	require.NotContains(t, before.String(), "pending: none")
	require.Contains(t, before.String(), "operator_to_staff_member.sql")

	var again bytes.Buffer
	require.NoError(t, printMigrationStatus(t.Context(), cfg, &again))
	require.Equal(t, before.String(), again.String())

	_, err := applyMigrations(t.Context(), cfg)
	require.NoError(t, err)
	var after bytes.Buffer
	require.NoError(t, printMigrationStatus(t.Context(), cfg, &after))
	require.Contains(t, after.String(), "pending: none")
}

// A start with auto-migrate waits for the lock for at most the timeout, fails, and leaves the
// schema untouched.
func TestMigrateOnStartFailsWhenTheLockIsHeldPastTheTimeout(t *testing.T) {
	cfg := emptyConfig(t, 1500*time.Millisecond)

	holder, err := sql.Open("pgx", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Close() })
	conn, err := holder.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	locker, err := lock.NewPostgresSessionLocker()
	require.NoError(t, err)
	require.NoError(t, locker.SessionLock(t.Context(), conn))

	require.Error(t, migrateOnStart(cfg))

	var out bytes.Buffer
	require.NoError(t, printMigrationStatus(t.Context(), cfg, &out))
	require.Contains(t, out.String(), "schema:  empty")
}
