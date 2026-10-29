package db_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/upgrade"
)

func emptyConfig(t *testing.T, autoMigrate bool) *config.Config {
	t.Helper()
	return &config.Config{
		DatabaseURL: testhelper.EmptyDBURL(t),
		AutoMigrate: autoMigrate,
		DBPool:      config.DBPool{MaxOpenConns: 2, MaxIdleConns: 2, ConnMaxLifetime: time.Minute, RiverMaxConns: 2},
	}
}

func statusText(t *testing.T, url string) string {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, db.WriteStatus(t.Context(), url, &out, false))
	return out.String()
}

// On an empty database ApplyMigrations creates the app schema, river's tables and the
// domain-event outbox, and is safe to run again.
func TestApplyMigrationsCreatesRiverAndOutboxSchemaOnEmptyDatabase(t *testing.T) {
	cfg := emptyConfig(t, false)

	_, err := db.ApplyMigrations(t.Context(), cfg.DatabaseURL)
	require.NoError(t, err)
	_, err = db.ApplyMigrations(t.Context(), cfg.DatabaseURL)
	require.NoError(t, err)

	pool, err := db.OpenRiverDB(cfg.DatabaseURL, cfg.DBPool)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(pool, "pool") })
	migrator, err := rivermigrate.New(riverdatabasesql.New(pool), nil)
	require.NoError(t, err)
	res, err := migrator.Validate(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, res.OK, "river migrations pending: %v", res.Messages)

	// The outbox the producer writes and the consumers read exists.
	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })
	client := db.NewEntClient(sqlDB)
	_, err = client.OutboxMessage.Query().Exist(t.Context())
	require.NoError(t, err)
	_, err = client.OutboxOffset.Query().Exist(t.Context())
	require.NoError(t, err)
}

// Without auto-migrate the start refuses a database that has not run the binary's migrations;
// with it, the start migrates, and the next start without it goes ahead.
func TestPrepareStartMigratesOnlyWhenAutoMigrateIsOn(t *testing.T) {
	guarded := emptyConfig(t, false)

	err := db.PrepareStart(t.Context(), guarded)

	var refusal *db.RefusalError
	require.ErrorAs(t, err, &refusal)
	refused := statusText(t, guarded.DatabaseURL)
	assert.NotContains(t, refused, "pending: none", "the guard changes nothing")

	migrating := &config.Config{DatabaseURL: guarded.DatabaseURL, AutoMigrate: true}
	err = db.PrepareStart(t.Context(), migrating)
	require.NoError(t, err)
	migrated := statusText(t, guarded.DatabaseURL)
	assert.Contains(t, migrated, "pending: none")
	err = db.PrepareStart(t.Context(), guarded)
	require.NoError(t, err)
}

// `migrate --status` prints the schema's state and the pending migrations and changes nothing.
func TestWriteStatusListsPendingAndChangesNothing(t *testing.T) {
	cfg := emptyConfig(t, false)

	before := statusText(t, cfg.DatabaseURL)
	require.Contains(t, before, "schema:  empty")
	require.Contains(t, before, "pending: ")
	require.NotContains(t, before, "pending: none")
	require.Contains(t, before, "_init.sql")

	again := statusText(t, cfg.DatabaseURL)
	require.Equal(t, before, again)

	_, err := db.ApplyMigrations(t.Context(), cfg.DatabaseURL)
	require.NoError(t, err)
	after := statusText(t, cfg.DatabaseURL)
	require.Contains(t, after, "pending: none")
	last := db.Majors[len(db.Majors)-1]
	require.Contains(t, after, fmt.Sprintf("state:   Major %d, oldest release that runs: %s (this binary: Major %d)", last.Major, last.Floor, last.Major))
}

// `migrate --status --json` is what `upgrade` reads from the release it is about to install: the
// database's state and the binary's table of Majors.
func TestWriteStatusJSONCarriesTheStateAndTheTableOfMajors(t *testing.T) {
	cfg := emptyConfig(t, false)

	var out bytes.Buffer
	require.NoError(t, db.WriteStatus(t.Context(), cfg.DatabaseURL, &out, true))

	var b upgrade.Boundaries
	require.NoError(t, json.Unmarshal(out.Bytes(), &b))
	require.Equal(t, 1, b.State.Major, "an empty database reports Major 1 until it is migrated")
	require.Len(t, b.Majors, len(db.Majors))
	require.Equal(t, db.Majors[len(db.Majors)-1].Major, b.Majors[len(b.Majors)-1].Major)
}
