package db_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/internal/infra/db"
)

// stepTreeVersion is the migration that gives every Step an id (ADR 0122).
const stepTreeVersion = "20261122135744"

// migrationsBefore holds the migrations that sort before version, the schema an Upgrade starts from.
func migrationsBefore(t *testing.T, version string) fs.FS {
	t.Helper()
	all, err := fs.Sub(sphericon.MigrationsFS, "migrations")
	require.NoError(t, err)
	entries, err := fs.ReadDir(all, ".")
	require.NoError(t, err)
	before := fstest.MapFS{}
	for _, e := range entries {
		if e.Name() >= version {
			continue
		}
		data, err := fs.ReadFile(all, e.Name())
		require.NoError(t, err)
		before[e.Name()] = &fstest.MapFile{Data: data}
	}
	return before
}

// The project carries no legacy (ADR 0047): a database holding an Automation version whose Steps
// have no ids is refused before a column goes, rather than losing its Enrollments' places.
func TestTheStepTreeMigrationRefusesADatabaseWithAutomations(t *testing.T) {
	sqlDB := openEmpty(t)
	_, err := db.MigrateFS(t.Context(), sqlDB, migrationsBefore(t, stepTreeVersion))
	require.NoError(t, err)
	// raw sql: the database has the schema before the migration under test, which the ent client
	// does not describe.
	_, err = sqlDB.ExecContext(t.Context(), `
		INSERT INTO workspaces (id, revision, name, public_id, collect_key) VALUES (1, 1, 'Acme', 'acme0001', 'acme0001');
		INSERT INTO automations (id, revision, name, workspace_id) VALUES (1, 1, 'Welcome', 1);
		INSERT INTO automation_versions (version, steps, trigger_event, automation_id, workspace_id)
		VALUES (1, '[{"type": "wait"}]', 'signed_up', 1, 1);`)
	require.NoError(t, err)

	_, err = db.Migrate(t.Context(), sqlDB)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reset it, the project carries no legacy")
}
