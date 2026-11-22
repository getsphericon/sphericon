package db_test

import (
	"path/filepath"
	"testing"

	"github.com/riverqueue/river"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// positionalHTTPStepArgs is the HTTP-step job as the release before queued it: by the step's
// position, unique by its args.
type positionalHTTPStepArgs struct {
	jobkind.Workspace

	RunID int64 `json:"run_id"`
	Step  int   `json:"step"`
}

func (positionalHTTPStepArgs) Kind() string { return jobkind.HTTPStep.Kind }

func (positionalHTTPStepArgs) InsertOpts() river.InsertOpts {
	return automations.RunHTTPStepArgs{}.InsertOpts()
}

// An Enrollment's position becomes the id of the Step at that position in its version (ADR
// 0122); one past the last Step names none (the Step records migration after it completes it).
// Every step job still to run names the id of the Step at its position, and an HTTP-step call
// queued by position is still the one call of its Step: the step job delivered again finds it and
// queues no second call.
func TestTheEnrollmentStepIDMigrationNamesEachEnrollmentsAndJobsStepByID(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "enrollment_step_id", filepath.Join("testdata", "enrollment_step_id"))
	rc := testhelper.RiverClientOn(t, sqlDB)
	queued, err := rc.Insert(t.Context(), positionalHTTPStepArgs{WorkspaceID: 1, RunID: 4}, nil)
	require.NoError(t, err)

	_, err = db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	runs := entclient.Core.New(sqlDB).AutomationRun
	stepOf := func(id int64) string { return lo.FromPtr(runs.GetX(t.Context(), id).CurrentStepID) }
	assert.Equal(t, "00000000-0000-4000-8000-000000000103", stepOf(1), "the Step at its position")
	assert.Empty(t, stepOf(2), "past the last Step: none")
	assert.Equal(t, automationrun.StatusCompleted, runs.GetX(t.Context(), 2).Status, "completed by the Step records migration after it")
	assert.Empty(t, stepOf(3))
	assert.Equal(t, "00000000-0000-4000-8000-000000000201", stepOf(4))

	left := queuedArgs(t, sqlDB)
	step := func(run int, id string) map[string]any {
		return map[string]any{"workspaceId": float64(1), "run_id": float64(run), "step_id": id}
	}
	assert.Equal(t, step(1, "00000000-0000-4000-8000-000000000103"), left[1])
	assert.Equal(t, step(4, "00000000-0000-4000-8000-000000000201"), left[2], "a first job names the first Step")
	assert.Equal(t, step(2, ""), left[3], "past the last Step: none")
	assert.Equal(t, step(99, ""), left[4], "an Enrollment gone: none")
	assert.Equal(t, step(1, "00000000-0000-4000-8000-000000000102"), left[5], "a job left running is retried by the next release")
	assert.Equal(t, map[string]any{"workspaceId": float64(1), "run_id": float64(1), "step": float64(0)}, left[6], "a finished job is left as it was")
	assert.Equal(t, step(4, "00000000-0000-4000-8000-000000000201"), left[queued.Job.ID])

	again, err := rc.Insert(t.Context(), automations.RunHTTPStepArgs{WorkspaceID: 1, RunID: 4, Step: "00000000-0000-4000-8000-000000000201"}, nil)
	require.NoError(t, err)
	assert.True(t, again.UniqueSkippedAsDuplicate, "the call queued by position is the Step's one call")
	assert.Equal(t, queued.Job.ID, again.Job.ID)
}
