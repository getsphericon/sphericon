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
		// raw sql: seeds the schema of a past release, whose columns the ent client no longer has.
		_, err := sqlDB.ExecContext(t.Context(),
			`INSERT INTO workspaces (name, slug, collect_key, created_at, updated_at) VALUES ($1, $1, $1, now(), now())`, name)
		require.NoError(t, err)
	}
}

// InsertDecidedRecipientWithoutMessage adds a Workspace and a Broadcast with one recipient decided
// as sent (id 1) and one still pending (id 2), neither with an Outbound message: the state of a
// database from before every recipient had its message. It writes the columns a later migration
// drops, which the ent client no longer has, so it talks SQL like InsertLegacyWorkspaces.
func InsertDecidedRecipientWithoutMessage(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO workspaces (id, name, public_id, collect_key, created_at, updated_at) VALUES (1, 'w', 'aaaaaaaa', 'k', now(), now())`,
		`INSERT INTO broadcasts (id, name, workspace_id, created_at, updated_at) VALUES (1, 'b', 1, now(), now())`,
		`INSERT INTO contacts (id, workspace_id, created_at, updated_at) VALUES (1, 1, now(), now()), (2, 1, now(), now())`,
		`INSERT INTO broadcast_recipients (id, broadcast_id, contact_id, status, workspace_id, created_at, updated_at)
		   VALUES (1, 1, 1, 'sent', 1, now(), now()), (2, 1, 2, 'pending', 1, now(), now())`,
	} {
		// raw sql: seeds the schema of a past release, whose columns the ent client no longer has.
		_, err := sqlDB.ExecContext(t.Context(), q)
		require.NoError(t, err, q)
	}
}

// InsertSentMessageForRecipient gives recipient 1 of InsertDecidedRecipientWithoutMessage its
// Outbound message.
func InsertSentMessageForRecipient(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	// raw sql: seeds the schema of a past release, whose columns the ent client no longer has.
	_, err := sqlDB.ExecContext(t.Context(),
		`INSERT INTO outbound_messages (kind, idempotency_key, status, claimed_at, broadcast_id, broadcast_recipient_id, workspace_id, created_at, updated_at)
		   VALUES ('broadcast', 'broadcast:1', 'sent', now(), 1, 1, 1, now(), now())`)
	require.NoError(t, err)
}

// ColumnsOf counts how many of the named columns the table has.
func ColumnsOf(t *testing.T, sqlDB *sql.DB, table string, columns ...string) int {
	t.Helper()
	n := 0
	for _, column := range columns {
		var found int
		// raw sql: reads information_schema, which is not an ent entity.
		require.NoError(t, sqlDB.QueryRowContext(t.Context(),
			`SELECT count(*) FROM information_schema.columns WHERE table_name = $1 AND column_name = $2`, table, column).Scan(&found))
		n += found
	}
	return n
}

// ColumnIsNullable reports whether the table's column allows NULL.
func ColumnIsNullable(t *testing.T, sqlDB *sql.DB, table, column string) bool {
	t.Helper()
	var nullable string
	// raw sql: reads information_schema, which is not an ent entity.
	require.NoError(t, sqlDB.QueryRowContext(t.Context(),
		`SELECT is_nullable FROM information_schema.columns WHERE table_name = $1 AND column_name = $2`, table, column).Scan(&nullable))
	return nullable == "YES"
}

// InsertLegacyAutomations adds a Workspace, two Contacts, and two Automations that keep their steps
// in the definition column: 1, with one Enrollment, and 2, with two. It is the state a database
// from before Automation versions is in, in a database migrated up to the migration that adds the
// versions table, so it writes the column the ent client no longer has and gives the Enrollments
// no version, and talks SQL like InsertLegacyWorkspaces.
func InsertLegacyAutomations(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO workspaces (id, name, public_id, collect_key, created_at, updated_at) VALUES (1, 'w', 'aaaaaaaa', 'k', now(), now())`,
		`INSERT INTO contacts (id, workspace_id, created_at, updated_at) VALUES (1, 1, now(), now()), (2, 1, now(), now())`,
		`INSERT INTO automations (id, name, status, trigger_event, definition, workspace_id, created_at, updated_at)
		   VALUES (1, 'one', 'active', 'a', '[{"type":"wait","seconds":1}]', 1, now(), now()),
		          (2, 'two', 'draft', 'b', '[{"type":"wait","seconds":2}]', 1, now(), now())`,
		`INSERT INTO automation_runs (id, automation_id, contact_id, workspace_id, created_at, updated_at)
		   VALUES (1, 1, 1, 1, now(), now()), (2, 2, 1, 1, now(), now()), (3, 2, 2, 1, now(), now())`,
	} {
		// raw sql: seeds the schema of a past release, whose columns the ent client no longer has.
		_, err := sqlDB.ExecContext(t.Context(), q)
		require.NoError(t, err, q)
	}
}
