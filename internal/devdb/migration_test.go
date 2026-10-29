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

func TestGenerateMigrationNeedsADiffDatabase(t *testing.T) {
	err := devdb.GenerateMigration(t.Context(), t.TempDir(), "", "next")

	require.ErrorContains(t, err, "DIFF_DB_URL is not set")
}
