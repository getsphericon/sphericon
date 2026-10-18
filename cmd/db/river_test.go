package main

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The generated migrations, applied by goose on an empty database, leave river with
// every one of its versions recorded: functions with dollar-quoted bodies included.
func TestGeneratedRiverMigrationsApplyAndRiverRecognisesThem(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, generateRiverMigration(dir))

	url := testhelper.EmptyDBURL(t)
	sqlDB, err := sql.Open("pgx", url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	applied, err := appdb.MigrateFS(t.Context(), sqlDB, os.DirFS(dir))
	require.NoError(t, err)
	require.NotZero(t, applied)

	pool, err := appdb.NewPGXPool(t.Context(), url, config.DBPool{MaxOpenConns: 2, MaxIdleConns: 2, ConnMaxLifetime: time.Minute, PGXMaxConns: 2})
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	require.NoError(t, err)
	res, err := migrator.Validate(t.Context(), nil)
	require.NoError(t, err)
	assert.True(t, res.OK, "river migrations pending: %v", res.Messages)
}

func TestRiverSQLDropsTheSchemaTemplate(t *testing.T) {
	assert.Equal(t, "CREATE TABLE river_job(id int);", riverSQL("CREATE TABLE /* TEMPLATE: schema */river_job(id int);"))
}

func TestRiverRecordMatchesTheShapeOfRiverMigrationAtThatVersion(t *testing.T) {
	assert.Equal(t, "INSERT INTO river_migration (version) VALUES (4);", riverRecord(4))
	assert.Equal(t, "INSERT INTO river_migration (line, version) VALUES ('main', 5);", riverRecord(5))
}
