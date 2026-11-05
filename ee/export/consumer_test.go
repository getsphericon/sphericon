package export_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/export"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/erasure"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// publishErased publishes the contact.erased of an Erasure in Initech.
func publishErased(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	require.NoError(t, env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(t.Context(), &events.ContactErased{
			WorkspaceID: fixtures.InitechID, SubjectID: "user-erased", IdentifierKind: "contact_id", RequesterKind: "user", RequesterID: 4,
		})
	}))
}

// erasures counts Initech's Erasures of the subject publishErased erases.
func erasures(t *testing.T, env *testhelper.TestEnv) int {
	t.Helper()
	n, err := env.DB.Erasure.Query().Where(erasure.WorkspaceID(fixtures.InitechID), erasure.SubjectID("user-erased")).Count(t.Context())
	require.NoError(t, err)
	return n
}

// An Erasure's contact.erased inserts one export job; working it adds the Erasure to the stream
// once, also when river retries the job.
func TestAnErasureReachesTheStreamOnceThroughItsJob(t *testing.T) {
	env := testhelper.Setup(t)
	publishErased(t, env)
	queued := env.EventJobs(t, export.ErasureEventArgs{}.Kind())
	require.Len(t, queued, 1)

	env.WorkEditionJobs(t)
	env.WorkEditionJobs(t)

	stored := erasures(t, env)
	assert.Equal(t, 1, stored)
}

// Without the export license the job is worked and stores nothing (ADR 0045).
func TestAnUnlicensedInstanceStreamsNoErasure(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	publishErased(t, env)
	queued := env.EventJobs(t, export.ErasureEventArgs{}.Kind())
	require.Len(t, queued, 1, "the job is queued whatever the license")

	env.WorkEditionJobs(t)

	stored := erasures(t, env)
	assert.Zero(t, stored)
}
