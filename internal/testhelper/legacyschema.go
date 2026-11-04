package testhelper

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// InsertLegacyWorkspace adds a Workspace to a database migrated only up to a Go migration under
// test, the state an Upgrade starts from, and returns its id. The ent client describes the newest
// schema and may write columns that database lacks, so the migration tests talk SQL, naming only
// the columns the row needs.
func InsertLegacyWorkspace(t *testing.T, sqlDB *sql.DB, publicID string) int64 {
	t.Helper()
	var id int64
	// raw sql: the database has the schema of an earlier release (frozen), see above.
	err := sqlDB.QueryRowContext(t.Context(),
		`INSERT INTO workspaces (name, public_id, collect_key) VALUES ($1, $1, $1) RETURNING id`, publicID).Scan(&id)
	require.NoError(t, err)
	return id
}

// InsertLegacyContact adds a Contact of workspaceID with the phone and the country (nil for none)
// as an earlier release stored them, and returns its id.
func InsertLegacyContact(t *testing.T, sqlDB *sql.DB, workspaceID int64, phone string, country *string) int64 {
	t.Helper()
	var id int64
	// raw sql: the database has the schema of an earlier release (frozen), see InsertLegacyWorkspace.
	err := sqlDB.QueryRowContext(t.Context(),
		`INSERT INTO contacts (workspace_id, phone, country) VALUES ($1, $2, $3) RETURNING id`,
		workspaceID, phone, country).Scan(&id)
	require.NoError(t, err)
	return id
}
