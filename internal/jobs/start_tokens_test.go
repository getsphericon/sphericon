package jobs_test

import (
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/starttoken"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The purge of Start tokens (ADR 0076) removes those that lapsed or were used, in every Workspace,
// and keeps the ones a link in an inbox can still redeem.
func TestThePurgeRemovesLapsedAndUsedStartTokensAndKeepsOpenOnes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	w := jobs.NewPruneStartTokensWorker(env.DB, time.Now)

	require.NoError(t, w.Work(ctx, &river.Job[jobs.PruneStartTokensArgs]{JobRow: &rivertype.JobRow{}}))

	left := env.DB.StartToken.Query().Where(starttoken.IDIn(
		fixtures.StartTokenAcmeUsedID, fixtures.StartTokenGlobexUsedID, fixtures.StartTokenAcmeOpenID,
		fixtures.StartTokenAcmeLapsedID, fixtures.StartTokenAcmeUsedOpenID,
	)).IDsX(ctx)
	assert.Equal(t, []int64{fixtures.StartTokenAcmeOpenID}, left)
}
