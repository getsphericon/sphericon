package testhelper

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// InsertLegacyWorkspaces adds a Workspace per name to a database migrated only up to the release
// before the Workspace id, the state an Upgrade starts from. The ent client cannot write that
// shape (its Workspace requires the id), so this one helper talks SQL for the migration tests.
func InsertLegacyWorkspaces(t *testing.T, sqlDB *sql.DB, names ...string) {
	t.Helper()
	for _, name := range names {
		_, err := sqlDB.ExecContext(t.Context(),
			`INSERT INTO workspaces (name, slug, collect_key, created_at, updated_at) VALUES ($1, $1, $1, now(), now())`, name)
		require.NoError(t, err)
	}
}
