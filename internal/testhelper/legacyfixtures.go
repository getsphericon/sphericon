package testhelper

import (
	"database/sql"
	"testing"

	"github.com/go-testfixtures/testfixtures/v3"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
)

// LoadLegacyFixtures loads the YAML files of dir into a database migrated only up to the
// release before one migration, the state an Upgrade starts from. A directory belongs to one
// migration (internal/db/testdata/legacy/<migration file name>) and its columns are the old
// schema's, which the ent client no longer has. It is the suite's fixture library (templated
// dates, load-time encryption, the loader) without the sequence reset to 100000: the old
// schema's sequences stay as the migrations made them, and the files' own ids are the rows'.
func LoadLegacyFixtures(t *testing.T, sqlDB *sql.DB, dir string) {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.NoError(t, loadFixtureDir(t.Context(), sqlDB, cfg, dir, testfixtures.SkipResetSequences()))
}
