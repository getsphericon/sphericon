package db_test

import (
	"database/sql"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sphericon "github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspaceid"
)

func openEmpty(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB, err := sql.Open("pgx", testhelper.EmptyDBURL(t))
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

// Two migrations started together run one after the other: the second finds nothing to do.
func TestMigrateStartedTwiceTogetherRunsOnceAndTheSecondFindsNothing(t *testing.T) {
	sqlDB := openEmpty(t)

	var wg sync.WaitGroup
	applied := make([]int, 2)
	errs := make([]error, 2)
	for i := range applied {
		wg.Go(func() { applied[i], errs[i] = db.Migrate(t.Context(), sqlDB) })
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Zero(t, min(applied[0], applied[1]))
	assert.Positive(t, max(applied[0], applied[1]))

	st, err := db.Status(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Empty(t, st.Pending)
	assert.Equal(t, st.Newest, st.Current)
}

// Status reads the schema and changes nothing.
func TestStatusOnAnEmptyDatabaseListsEveryMigrationAndLeavesItEmpty(t *testing.T) {
	sqlDB := openEmpty(t)

	st, err := db.Status(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Zero(t, st.Current)
	assert.Positive(t, st.Newest)
	assert.NotEmpty(t, st.Pending)

	again, err := db.Status(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, st, again)

	applied, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Len(t, st.Pending, applied)
}

// Migrate has no time limit of its own: it applies a migration under a context with no
// deadline, and the lock wait is bounded by that context alone.
func TestMigrateFSAppliesAMigrationUnderAContextWithNoDeadline(t *testing.T) {
	sqlDB := openEmpty(t)
	files := fstest.MapFS{"20200101000000_slow.sql": {Data: []byte("-- +goose Up\nCREATE TABLE slow_done (id int);\n")}}
	_, hasDeadline := t.Context().Deadline()
	require.False(t, hasDeadline)

	applied, err := db.MigrateFS(t.Context(), sqlDB, files)
	require.NoError(t, err)
	assert.Equal(t, 1, applied)

	st, err := db.StatusFS(t.Context(), sqlDB, files)
	require.NoError(t, err)
	assert.Empty(t, st.Pending)
}

// An Instance upgraded from before the Workspace id gets one for every Workspace it already had,
// and the column ends required and unique (ADR 0043).
func TestMigrateGivesExistingWorkspacesAnIdAndEndsRequired(t *testing.T) {
	sqlDB := openEmpty(t)
	before := migrationsWhere(t, func(name string) bool { return name < "20261020030001" })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.InsertLegacyWorkspaces(t, sqlDB, "one", "two", "three")

	_, err = db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
	t.Cleanup(func() { _ = client.Close() })
	workspaces, err := client.Workspace.Query().All(t.Context())
	require.NoError(t, err)
	require.Len(t, workspaces, 3)
	seen := map[string]bool{}
	for _, ws := range workspaces {
		assert.Regexp(t, workspaceid.Pattern, ws.PublicID)
		seen[ws.PublicID] = true
	}
	assert.Len(t, seen, 3, "every id is its own")

	// The ids are unique now: a fourth Workspace cannot take one of them.
	err = client.Workspace.Create().SetName("y").SetCollectKey("y").SetPublicID(workspaces[0].PublicID).Exec(t.Context())
	assert.True(t, db.IsUniqueViolation(err))
}

// Dropping the slug refuses an Instance whose Workspaces have not all got their id yet: it would
// leave them with no address (ADR 0033 guard).
func TestMigrateRefusesToDropTheSlugBeforeEveryWorkspaceHasAnId(t *testing.T) {
	sqlDB := openEmpty(t)
	// Skip the id backfill and the migration that requires the id.
	skipping := migrationsWhere(t, func(name string) bool { return name < "20261020030002" || name >= "20261020030004" })
	before := migrationsWhere(t, func(name string) bool { return name < "20261020030002" })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.InsertLegacyWorkspaces(t, sqlDB, "one")

	_, err = db.MigrateFS(t.Context(), sqlDB, skipping)
	require.ErrorContains(t, err, "workspaces.slug cannot be dropped")
}

// migrationsWhere is the embedded migrations whose file name satisfies keep.
func migrationsWhere(t *testing.T, keep func(name string) bool) fstest.MapFS {
	t.Helper()
	all, err := fs.Sub(sphericon.MigrationsFS, "migrations")
	require.NoError(t, err)
	entries, err := fs.ReadDir(all, ".")
	require.NoError(t, err)
	out := fstest.MapFS{}
	for _, e := range entries {
		if keep(e.Name()) {
			data, err := fs.ReadFile(all, e.Name())
			require.NoError(t, err)
			out[e.Name()] = &fstest.MapFile{Data: data}
		}
	}
	return out
}

// Dropping the recipient's own outcome refuses a Broadcast recipient that was decided before its
// Outbound message existed: the outcome would be lost with the column (ADR 0033 guard). A pending
// recipient has nothing to lose, and once the message exists the migration goes through.
func TestMigrateRefusesToDropARecipientOutcomeThatHasNoOutboundMessage(t *testing.T) {
	const dropping = "20261020185327_drop_recipient_outcome_and_broadcast_counters.sql"
	sqlDB := openEmpty(t)
	before := migrationsWhere(t, func(name string) bool { return name < dropping })
	all := migrationsWhere(t, func(name string) bool { return name <= dropping })
	_, err := db.MigrateFS(t.Context(), sqlDB, before)
	require.NoError(t, err)
	testhelper.InsertDecidedRecipientWithoutMessage(t, sqlDB)

	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.ErrorContains(t, err, "would lose their outcome")

	testhelper.InsertSentMessageForRecipient(t, sqlDB)
	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.NoError(t, err)

	columns := testhelper.ColumnsOf(t, sqlDB, "broadcast_recipients", "status", "error", "sent_at", "outbound_message_id")
	assert.Zero(t, columns, "the recipient's own outcome is gone")
}

// Dropping automations.definition refuses an Instance whose Automations have not all been given a
// version yet (the river job backfill_automation_versions): the steps would be lost (ADR 0049).
func TestMigrateRefusesToDropTheDefinitionBeforeTheBackfill(t *testing.T) {
	sqlDB := openEmpty(t)
	legacy := migrationsWhere(t, func(name string) bool { return name < "20261020191942" })
	_, err := db.MigrateFS(t.Context(), sqlDB, legacy)
	require.NoError(t, err)
	testhelper.InsertLegacyAutomations(t, sqlDB)
	all := migrationsWhere(t, func(string) bool { return true })
	beforeDrop := migrationsWhere(t, func(name string) bool { return name < "20261020191944" })
	_, err = db.MigrateFS(t.Context(), sqlDB, beforeDrop)
	require.NoError(t, err)

	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.ErrorContains(t, err, "automations.definition cannot be dropped")
	assert.Equal(t, 1, testhelper.ColumnsOf(t, sqlDB, "automations", "definition"), "the column is still there")
}

// The backfill job gives every Automation made before versions its version 1, holding the steps it
// had, and points its Enrollments at it; only then does the migration drop the column.
func TestBackfillGivesEveryAutomationVersionOneAndMovesItsEnrollments(t *testing.T) {
	sqlDB := openEmpty(t)
	legacy := migrationsWhere(t, func(name string) bool { return name < "20261020191942" })
	_, err := db.MigrateFS(t.Context(), sqlDB, legacy)
	require.NoError(t, err)
	testhelper.InsertLegacyAutomations(t, sqlDB)
	all := migrationsWhere(t, func(string) bool { return true })
	beforeDrop := migrationsWhere(t, func(name string) bool { return name < "20261020191944" })
	_, err = db.MigrateFS(t.Context(), sqlDB, beforeDrop)
	require.NoError(t, err)
	client := db.NewEntClient(sqlDB)
	t.Cleanup(func() { _ = client.Close() })

	// A batch of one makes the job loop, and a second run finds nothing left.
	made, pointed, err := jobs.BackfillAutomationVersions(t.Context(), client, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, made)
	assert.Equal(t, 3, pointed)
	made, pointed, err = jobs.BackfillAutomationVersions(t.Context(), client, 1)
	require.NoError(t, err)
	assert.Zero(t, made+pointed)

	_, err = db.MigrateFS(t.Context(), sqlDB, all)
	require.NoError(t, err, "the guard lets the drop through once the data has moved")
	assert.Zero(t, testhelper.ColumnsOf(t, sqlDB, "automations", "definition"))

	versions, err := client.AutomationVersion.Query().All(t.Context())
	require.NoError(t, err)
	steps := map[int64]string{}
	for _, v := range versions {
		assert.Equal(t, 1, v.Version)
		steps[v.AutomationID] = v.Steps
	}
	assert.Equal(t, map[int64]string{1: `[{"type":"wait","seconds":1}]`, 2: `[{"type":"wait","seconds":2}]`}, steps)
	runs, err := client.AutomationRun.Query().All(t.Context())
	require.NoError(t, err)
	require.Len(t, runs, 3)
	versionOf := map[int64]int64{}
	for _, v := range versions {
		versionOf[v.AutomationID] = v.ID
	}
	for _, r := range runs {
		assert.Equal(t, versionOf[r.AutomationID], r.VersionID, "run %d", r.ID)
	}

	// After the drop the job still plans (the column is read as JSON) and finds nothing to do.
	made, pointed, err = jobs.BackfillAutomationVersions(t.Context(), client, 1)
	require.NoError(t, err)
	assert.Zero(t, made+pointed)
}
