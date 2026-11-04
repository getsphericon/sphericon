package testhelper

import (
	"database/sql"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	sphericon "github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// SchemaBefore is the input of a migration under test: a database of the test's own, migrated
// with every committed migration older than the one whose file name ends in "_<name>.sql" (the Go
// ones of internal/db included), and
// loaded with the fixture files of dir, rows in the shape of that older schema (review rule 10:
// the shared fixtures hold today's shape only). The test runs db.Migrate on it to apply the
// migration and the ones after it, then reads the result. It is dropped with the test.
func SchemaBefore(t *testing.T, name, dir string) *sql.DB {
	t.Helper()
	ctx := t.Context()
	sqlDB, err := sql.Open("pgx", EmptyDBURL(t))
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "schema before "+name) })

	all, err := fs.Sub(sphericon.MigrationsFS, "migrations")
	require.NoError(t, err)
	files, err := fs.ReadDir(all, ".")
	require.NoError(t, err)
	older := fstest.MapFS{}
	var version int64
	for _, f := range files {
		if strings.HasSuffix(f.Name(), "_"+name+".sql") {
			version, err = strconv.ParseInt(strings.SplitN(f.Name(), "_", 2)[0], 10, 64)
			require.NoError(t, err)
			break
		}
		data, err := fs.ReadFile(all, f.Name())
		require.NoError(t, err)
		older[f.Name()] = &fstest.MapFile{Data: data}
	}
	require.NotZerof(t, version, "no migration named %q", name)
	_, err = db.MigrateBefore(ctx, sqlDB, older, version)
	require.NoError(t, err)

	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	require.NoError(t, fixtures.Load(ctx, sqlDB, cipher, dir))
	return sqlDB
}
