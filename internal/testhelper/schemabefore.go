package testhelper

import (
	"database/sql"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	sphericon "github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
)

// SchemaBefore is the input of a migration under test (ADR 0047): a database of the test's own,
// migrated with every committed migration older than the one whose file name ends in
// "_<name>.sql", and loaded with the fixture files of dir, rows in the shape of that older schema
// (review rule 10: the shared fixtures hold today's shape only). The test runs db.Migrate on it to
// apply the migration and the ones after it, then reads the result. It is dropped with the test.
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
	found := false
	for _, f := range files {
		if strings.HasSuffix(f.Name(), "_"+name+".sql") {
			found = true
			break
		}
		data, err := fs.ReadFile(all, f.Name())
		require.NoError(t, err)
		older[f.Name()] = &fstest.MapFile{Data: data}
	}
	require.Truef(t, found, "no migration named %q", name)
	_, err = db.MigrateFS(ctx, sqlDB, older)
	require.NoError(t, err)

	require.NoError(t, fixtures.Load(ctx, sqlDB, Cipher(t), dir))
	return sqlDB
}
