package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/jobs"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	entusage "github.com/getsphericon/sphericon/ent/usage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestTheRunnerRegistersTheAbuseDetectorNextToTheOtherJobs(t *testing.T) {
	ext := jobs.Extension(jobs.Deps{})

	assert.Len(t, ext.PeriodicJobs, 4, "retention, usage, the abuse detector and the Erasures stream's prune")
	assert.NotPanics(t, func() { ext.Workers(river.NewWorkers()) })
}

var errBoom = errors.New("boom")

// tick queues the periodic job args and works it: the jobs it fans out into wait in the queue.
func tick(t *testing.T, env *testhelper.TestEnv, args river.JobArgs) {
	t.Helper()
	env.EnqueueJob(t, args)
	env.PerformEnqueuedJobs(t, args)
}

// stateOf is the state of the job of kind queued for the Workspace ws, failing when there is none.
func stateOf(t *testing.T, env *testhelper.TestEnv, kind string, ws int64) rivertype.JobState {
	t.Helper()
	row, ok := lo.Find(env.JobRowsOf(t, kind), func(r *rivertype.JobRow) bool {
		var args struct {
			WorkspaceID int64 `json:"workspaceId"`
		}
		return json.Unmarshal(r.EncodedArgs, &args) == nil && args.WorkspaceID == ws
	})
	require.Truef(t, ok, "no %s job for workspace %d", kind, ws)
	return row.State
}

// failingRows is an ent hook that fails an update or delete of the type's rows that include id.
func failingRows(typ string, id int64) ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if rows, ok := m.(interface {
				IDs(ctx context.Context) ([]int64, error)
			}); ok && m.Type() == typ && m.Op().Is(ent.OpUpdate|ent.OpUpdateOne|ent.OpDelete|ent.OpDeleteOne) {
				ids, err := rows.IDs(ctx)
				if err == nil && slices.Contains(ids, id) {
					return nil, errBoom
				}
			}
			return next.Mutate(ctx, m)
		})
	}
}

// failingWorkspace is an ent hook that fails every change that names the Workspace ws.
func failingWorkspace(ws int64) ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if named, ok := m.(interface{ WorkspaceID() (int64, bool) }); ok {
				if id, set := named.WorkspaceID(); set && id == ws {
					return nil, errBoom
				}
			}
			return next.Mutate(ctx, m)
		})
	}
}

// The usage tick queues a refresh per Workspace; Acme's fails and stays to retry, Globex's
// refreshes its own figures.
func TestTheUsageTickRefreshesEachWorkspaceInAJobOfItsOwn(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return now }))
	env.DB.Usage.Use(failingWorkspace(fixtures.AcmeID))
	kind := jobs.WorkspaceUsageRefreshArgs{}.Kind()

	tick(t, env, jobs.UsageRefreshArgs{})
	env.TryPerformEnqueuedJobs(t, jobs.WorkspaceUsageRefreshArgs{})

	refreshed := func(ws int64) int {
		return env.DB.Scoped(ws).Usage().Query().Where(entusage.ComputedAt(now)).CountX(t.Context())
	}
	assert.Equal(t, rivertype.JobStateRetryable, stateOf(t, env, kind, fixtures.AcmeID))
	assert.Zero(t, refreshed(fixtures.AcmeID))
	assert.Equal(t, rivertype.JobStateCompleted, stateOf(t, env, kind, fixtures.GlobexID))
	assert.Positive(t, refreshed(fixtures.GlobexID))
}

// The abuse detector's tick queues a judgement per Workspace: Kramerica's freeze fails and stays
// to retry, Vandelay is frozen all the same.
func TestTheAbuseTickJudgesEachWorkspaceInAJobOfItsOwn(t *testing.T) {
	overDay := time.Now().Add(-3 * 24 * time.Hour) // Vandelay at 15% complaints, Kramerica at 50%
	env := testhelper.Setup(t, testhelper.WithStaffVolumeFloor(20), testhelper.WithClock(func() time.Time { return overDay }))
	env.DB.Workspace.UpdateOneID(fixtures.KramericaID).SetAbuseExempt(false).ExecX(t.Context())
	// The suspension is written in a transaction of the bus, whose client runs the bus's hooks.
	env.Bus.Use(failingRows(ent.TypeWorkspace, fixtures.KramericaID))
	kind := jobs.WorkspaceAbuseDetectionArgs{}.Kind()

	tick(t, env, jobs.AbuseDetectionArgs{})
	env.TryPerformEnqueuedJobs(t, jobs.WorkspaceAbuseDetectionArgs{})

	suspended := func(ws int64) bool { return env.DB.Workspace.GetX(t.Context(), ws).SuspendedAt != nil }
	assert.Equal(t, rivertype.JobStateRetryable, stateOf(t, env, kind, fixtures.KramericaID))
	assert.False(t, suspended(fixtures.KramericaID))
	assert.Equal(t, rivertype.JobStateCompleted, stateOf(t, env, kind, fixtures.VandelayID))
	assert.True(t, suspended(fixtures.VandelayID))
}

// juneFirst is past the retention windows below and ClientWindow for every fixture Audit entry
// but Globex's youngest.
var juneFirst = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func entries(t *testing.T, env *testhelper.TestEnv, ws int64) int {
	t.Helper()
	return env.DB.Scoped(ws).AuditEntry().Query().CountX(t.Context())
}

// The retention tick queues a sweep per Workspace: Acme's prune fails and stays to retry, Globex
// prunes its own log.
func TestTheRetentionTickPrunesEachWorkspaceInAJobOfItsOwn(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return juneFirst }))
	env.DB.Workspace.Update().SetRetentionDays(7).ExecX(t.Context())
	env.DB.AuditEntry.Use(failingRows(ent.TypeAuditEntry, fixtures.AcmeAuditEntryID))
	kind := jobs.WorkspaceRetentionPruneArgs{}.Kind()

	tick(t, env, jobs.RetentionPruneArgs{})
	env.TryPerformEnqueuedJobs(t, jobs.WorkspaceRetentionPruneArgs{})

	assert.Equal(t, rivertype.JobStateRetryable, stateOf(t, env, kind, fixtures.AcmeID))
	assert.Equal(t, 1, entries(t, env, fixtures.AcmeID))
	assert.Equal(t, rivertype.JobStateCompleted, stateOf(t, env, kind, fixtures.GlobexID))
	assert.Zero(t, entries(t, env, fixtures.GlobexID))
}

// The retention tick clears the client details of each Workspace in a job of its own: Acme's
// fails and stays to retry, Globex's entries lose theirs.
func TestTheRetentionTickForgetsTheClientsOfEachWorkspaceInAJobOfItsOwn(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return juneFirst }))
	env.DB.AuditEntry.UpdateOneID(fixtures.AcmeAuditEntryID).SetIP("10.0.0.9").ExecX(t.Context())
	env.DB.AuditEntry.Use(failingRows(ent.TypeAuditEntry, fixtures.AcmeAuditEntryID))
	kind := jobs.WorkspaceRetentionPruneArgs{}.Kind()

	tick(t, env, jobs.RetentionPruneArgs{})
	env.TryPerformEnqueuedJobs(t, jobs.WorkspaceRetentionPruneArgs{})

	ip := func(id int64) *string {
		return env.DB.AuditEntry.Query().Where(auditentry.ID(id)).OnlyX(t.Context()).IP
	}
	assert.Equal(t, rivertype.JobStateRetryable, stateOf(t, env, kind, fixtures.AcmeID))
	assert.NotNil(t, ip(fixtures.AcmeAuditEntryID))
	assert.Equal(t, rivertype.JobStateCompleted, stateOf(t, env, kind, fixtures.GlobexID))
	assert.Nil(t, ip(fixtures.GlobexAuditIntegrationUpdateID))
}
