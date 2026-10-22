package eligibility_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The check reads the scoped Workspace only: another Workspace's Suppression never
// blocks, and a Workspace with no Suppression at all still gets a decision.
func TestCheckIsConfinedToTheScopedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.GlobexID), eligibility.ChannelEmail,
		fixtures.SuppressionGlobexDestination, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonSuppressed, d.Reason)

	d, err = eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		fixtures.SuppressionGlobexDestination, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.True(t, d.Eligible, "a Globex suppression does not reach Acme")

	d, err = eligibility.Check(ctx, env.DB.Scoped(fixtures.HooliID), eligibility.ChannelEmail,
		"anyone@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.True(t, d.Eligible, "a Workspace without Suppression rows is still decided")
}
