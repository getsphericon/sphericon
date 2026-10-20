package eligibility_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestCheckEmptyDestinationIsEligible(t *testing.T) {
	env := testhelper.Setup(t)
	d, err := eligibility.Check(context.Background(), env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"   ", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.True(t, d.Eligible, "nothing to suppress against")
}

func TestCheckFailsClosedOnStoreErrorAndUnknownWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail, "a@example.com", eligibility.SourceBroadcasts)
	require.ErrorContains(t, err, "eligibility check")

	_, err = eligibility.Check(context.Background(), env.DB.Scoped(987654), eligibility.ChannelEmail, "a@example.com", eligibility.SourceBroadcasts)
	assert.ErrorContains(t, err, "workspace 987654 not found")
}

// GloballyOptedOut matches suppressed and unsubscribed-from-everything contacts,
// but not a per-source opt-out and not a clean contact.
func TestGloballyOptedOut(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	mk := func(email string) int64 {
		return env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail(email).SaveX(ctx).ID
	}
	suppressed, everything, perSource, clean := mk("Sup@Example.com"), mk("all@example.com"), mk("some@example.com"), mk("ok@example.com")

	env.DB.Suppression.Create().SetWorkspaceID(fixtures.AcmeID).SetChannel(suppression.ChannelEmail).
		SetDestination("sup@example.com").SetReason(suppression.ReasonBounce).ExecX(ctx)
	env.DB.Unsubscribe.Create().SetWorkspaceID(fixtures.AcmeID).SetChannel(unsubscribe.ChannelEmail).
		SetDestination("all@example.com").SetSendingSource(eligibility.SourceEverything).ExecX(ctx)
	env.DB.Unsubscribe.Create().SetWorkspaceID(fixtures.AcmeID).SetChannel(unsubscribe.ChannelEmail).
		SetDestination("some@example.com").SetSendingSource(eligibility.SourceBroadcasts).ExecX(ctx)

	ids := env.DB.Contact.Query().
		Where(contact.IDIn(suppressed, everything, perSource, clean), eligibility.GloballyOptedOut(eligibility.ChannelEmail)).
		IDsX(ctx)
	assert.ElementsMatch(t, []int64{suppressed, everything}, ids)
}

func TestParseAutomationSource(t *testing.T) {
	id, ok := eligibility.ParseAutomationSource(eligibility.AutomationSource(42))
	assert.True(t, ok)
	assert.Equal(t, int64(42), id)

	for _, src := range []string{"broadcasts", "everything", "", "automation:", "automation:abc", "automation:1.5", "Automation:1"} {
		_, ok := eligibility.ParseAutomationSource(src)
		assert.False(t, ok, "%q is not an automation scope", src)
	}
}
