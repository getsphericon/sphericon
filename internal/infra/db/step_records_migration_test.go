package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A wait holds the Enrollment on itself (ADR 0124): an Enrollment the release before parked on the
// Step after a wait, or past the last Step after a wait that is the last, moves to the wait,
// keeping its resume moment, and its queued job names the wait. One still past the last Step with
// no wait completes, and its job is left to find it ended. Every active Enrollment gets the record
// of the Step it stands on, so the move that leaves it stamps it.
func TestTheStepRecordsMigrationMovesParkedEnrollmentsOntoTheirWaits(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "step_records", filepath.Join("testdata", "step_records"))

	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	client := entclient.Core.New(sqlDB)
	runs := client.AutomationRun
	stepOf := func(id int64) string { return lo.FromPtr(runs.GetX(t.Context(), id).CurrentStepID) }
	resumeAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "00000000-0000-4000-8000-000000000102", stepOf(1), "on the wait it was parked after")
	assert.WithinDuration(t, resumeAt, lo.FromPtr(runs.GetX(t.Context(), 1).ResumeAt), 0, "due when it was to resume")
	assert.Equal(t, "00000000-0000-4000-8000-000000000302", stepOf(2), "on the wait that is the last Step")
	assert.Equal(t, automationrun.StatusActive, runs.GetX(t.Context(), 2).Status)
	assert.Equal(t, automationrun.StatusCompleted, runs.GetX(t.Context(), 3).Status, "past the last Step with no wait: completed")
	assert.Equal(t, "00000000-0000-4000-8000-000000000101", stepOf(4), "an Enrollment on an email stays")
	assert.Equal(t, "00000000-0000-4000-8000-000000000103", stepOf(5), "a finished Enrollment stays")

	left := queuedArgs(t, sqlDB)
	step := func(run int, id string) map[string]any {
		return map[string]any{"workspaceId": float64(1), "run_id": float64(run), "step_id": id}
	}
	assert.Equal(t, step(1, "00000000-0000-4000-8000-000000000102"), left[1], "the job at the resume moment names the wait")
	assert.Equal(t, step(2, "00000000-0000-4000-8000-000000000302"), left[2])
	assert.Equal(t, step(3, ""), left[3], "left to find its Enrollment ended")
	assert.Equal(t, step(1, "00000000-0000-4000-8000-000000000103"), left[4], "a finished job is left as it was")

	records := client.AutomationStepRecord.Query().Order(ent.Asc(automationsteprecord.FieldRunID)).AllX(t.Context())
	assert.Equal(t, []int64{1, 2, 4}, lo.Map(records, func(r *ent.AutomationStepRecord, _ int) int64 { return r.RunID }), "a record for each active Enrollment")
	assert.Equal(t, []string{"00000000-0000-4000-8000-000000000102", "00000000-0000-4000-8000-000000000302", "00000000-0000-4000-8000-000000000101"},
		lo.Map(records, func(r *ent.AutomationStepRecord, _ int) string { return r.StepID }))
	assert.WithinDuration(t, resumeAt, lo.FromPtr(records[0].DueAt), 0, "a wait's record is due at the resume moment")
	assert.Nil(t, records[2].DueAt, "an email has no due moment")
	assert.WithinDuration(t, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), records[2].ReachedAt, 0, "reached when its cursor last moved")
	assert.Equal(t, []bool{true, true, true}, lo.Map(records, func(r *ent.AutomationStepRecord, _ int) bool { return r.LeftAt == nil }), "none is left yet")
}
