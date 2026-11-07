package db_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// queuedArgs is every job left in the queue, by its id, with its args, read through river.
func queuedArgs(t *testing.T, sqlDB *sql.DB) map[int64]map[string]any {
	t.Helper()
	rc, err := river.NewClient(riverdatabasesql.New(sqlDB), &river.Config{})
	require.NoError(t, err)
	res, err := rc.JobList(t.Context(), river.NewJobListParams().First(100))
	require.NoError(t, err)
	return lo.SliceToMap(res.Jobs, func(job *rivertype.JobRow) (int64, map[string]any) {
		var args map[string]any
		require.NoError(t, json.Unmarshal(job.EncodedArgs, &args))
		return job.ID, args
	})
}

// The trigger and rule evaluations an earlier release queued name their Workspace at workspaceId
// after the migration, a running one is left alone, and the jobs that name a Workspace an earlier
// release erased are gone, but for a running one. A job that names only a row stays.
func TestTheWorkspaceJobArgsMigrationRenamesTheKeyAndDropsTheErasedWorkspacesJobs(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "workspace_job_args", filepath.Join("testdata", "workspace_job_args"))

	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	left := queuedArgs(t, sqlDB)
	assert.ElementsMatch(t, []int64{1, 2, 3, 6, 7, 8}, lo.Keys(left))
	assert.Equal(t, map[string]any{"workspaceId": float64(1), "contact_id": float64(5), "action": "page_view"}, left[1])
	assert.Equal(t, map[string]any{"workspaceId": float64(1), "contact_id": float64(5), "action": "page_view", "source_id": "evt-1"}, left[2])
	assert.Equal(t, map[string]any{"workspace_id": float64(1), "contact_id": float64(6), "action": "page_view"}, left[3], "a running job keeps the key its release reads")
	assert.Equal(t, map[string]any{"recipient_id": float64(9), "broadcast_id": float64(3)}, left[8])
}
