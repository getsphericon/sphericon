package devdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/devdb"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// ent's diff engine refuses a directory that has migrations and no atlas.sum, and writes one after
// the migration it plans; goose has no use for the file, so the generator works without one and
// leaves none behind.
func TestGenerateMigrationWritesTheNextMigrationAndNoIntegrityFile(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "20260101000000_a.sql"),
		[]byte("-- +goose Up\nSELECT 1;\n\n-- +goose Down\nSELECT 2;\n"), 0o644)
	require.NoError(t, err)
	diffURL := testhelper.EmptyDBURL(t)

	err = devdb.GenerateMigration(t.Context(), dir, diffURL, "next")

	require.NoError(t, err)
	written, err := filepath.Glob(filepath.Join(dir, "*_next.sql"))
	require.NoError(t, err)
	assert.Len(t, written, 1)
	assert.NoFileExists(t, filepath.Join(dir, "atlas.sum"))
}

// goose runs each migration in a transaction of its own, and a migration may rely on that: LOCK
// TABLE is refused outside one.
func TestGenerateMigrationRunsEachMigrationInItsTransaction(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "20260101000000_a.sql"),
		[]byte("-- +goose Up\nCREATE TABLE a (id int);\nLOCK TABLE a IN ACCESS EXCLUSIVE MODE;\n\n-- +goose Down\nDROP TABLE a;\n"), 0o644)
	require.NoError(t, err)
	diffURL := testhelper.EmptyDBURL(t)

	err = devdb.GenerateMigration(t.Context(), dir, diffURL, "next")

	require.NoError(t, err)
}

// The committed migrations are the ent schema, so there is nothing to generate from them, again
// and again on one diff database: what a run leaves in it (river's types, goose's version table)
// does not break the next one, and the ent schema is compared the way Postgres stores it (an
// index predicate's `IN (...)` read back as `= ANY (ARRAY[...])` is no change).
func TestGenerateMigrationFindsNothingToGenerateFromTheCommittedMigrations(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "migrations"))))
	diffURL := testhelper.EmptyDBURL(t)

	for _, name := range []string{"first", "again"} {
		err := devdb.GenerateMigration(t.Context(), dir, diffURL, name)

		require.NoError(t, err)
		written, err := filepath.Glob(filepath.Join(dir, "*_"+name+".sql"))
		require.NoError(t, err)
		assert.Empty(t, written)
	}
}

func TestGenerateMigrationNeedsADiffDatabase(t *testing.T) {
	err := devdb.GenerateMigration(t.Context(), t.TempDir(), "", "next")

	require.ErrorContains(t, err, "DIFF_DB_URL is not set")
}
