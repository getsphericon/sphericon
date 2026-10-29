package devdb_test

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/cleanup"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/devdb"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The generated migrations, applied by goose on an empty database, leave river with
// every one of its versions recorded: functions with dollar-quoted bodies included.
func TestGeneratedRiverMigrationsApplyAndRiverRecognisesThem(t *testing.T) {
	dir := t.TempDir()
	err := devdb.GenerateRiverMigration(dir, io.Discard)
	require.NoError(t, err)

	url := testhelper.EmptyDBURL(t)
	sqlDB, err := sql.Open("pgx", url)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })
	applied, err := appdb.MigrateFS(t.Context(), sqlDB, os.DirFS(dir))
	require.NoError(t, err)
	require.NotZero(t, applied)

	migrator, err := rivermigrate.New(riverdatabasesql.New(sqlDB), nil)
	require.NoError(t, err)
	res, err := migrator.Validate(t.Context(), nil)
	require.NoError(t, err)
	assert.True(t, res.OK, "river migrations pending: %v", res.Messages)
}

// River's schema template is a comment the SQL would run with, so only the text shows it is gone.
func TestGeneratedRiverMigrationsCarryNoSchemaTemplate(t *testing.T) {
	dir := t.TempDir()

	err := devdb.GenerateRiverMigration(dir, io.Discard)

	require.NoError(t, err)
	files, err := filepath.Glob(filepath.Join(dir, "*_river_*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, name := range files {
		text, err := os.ReadFile(name)
		require.NoError(t, err)
		assert.NotContains(t, string(text), "TEMPLATE: schema", name)
	}
}

func TestRiverMigrationOfAnUpToDateDirectoryWritesNothing(t *testing.T) {
	dir := t.TempDir()
	err := devdb.GenerateRiverMigration(dir, io.Discard)
	require.NoError(t, err)
	before, err := os.ReadDir(dir)
	require.NoError(t, err)

	err = devdb.GenerateRiverMigration(dir, io.Discard)

	require.NoError(t, err)
	after, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, after, len(before))
}
