package auditentries_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// recordEntry records an Audit entry of a User in Initech through the seam, as an audited change
// does.
func recordEntry(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	require.NoError(t, env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		return events.RecordAudit(t.Context(), pub, &events.AuditEntry{
			WorkspaceID: fixtures.InitechID, Actor: events.Actor{Kind: events.ActorUser, ID: "7", Name: "Mary"},
			Action: "segment.update", TargetType: "segment", TargetID: "11",
		})
	}))
}

// entries counts Initech's stored Audit entries of the change recordEntry records.
func entries(t *testing.T, env *testhelper.TestEnv) int {
	t.Helper()
	n, err := env.DB.AuditEntry.Query().
		Where(auditentry.WorkspaceID(fixtures.InitechID), auditentry.Action("segment.update"), auditentry.TargetID("11")).
		Count(t.Context())
	require.NoError(t, err)
	return n
}

// An audited change inserts one audit job; working it stores the Audit entry once, also when
// river retries the job.
func TestAnAuditedChangeIsStoredOnceThroughItsJob(t *testing.T) {
	env := testhelper.Setup(t)
	recordEntry(t, env)
	queued := env.EventJobs(t, auditentries.EntryEventArgs{}.Kind())
	require.Len(t, queued, 1)

	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	env.RetryJobs(t, testhelper.EditionJobs...)
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	stored := entries(t, env)
	assert.Equal(t, 1, stored)
}

// Without the audit license the job is worked and stores nothing.
func TestAnUnlicensedInstanceStoresNoAuditEntry(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	recordEntry(t, env)
	queued := env.EventJobs(t, auditentries.EntryEventArgs{}.Kind())
	require.Len(t, queued, 1, "the job is queued whatever the license")

	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	stored := entries(t, env)
	assert.Zero(t, stored)
}
