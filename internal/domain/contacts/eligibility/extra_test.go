package eligibility_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An empty destination is eligible on every Channel, even where a Confirmation is asked for (email
// in Lumon, sms anywhere): there is nothing to suppress or confirm against.
func TestCheckEmptyDestinationIsEligible(t *testing.T) {
	env := testhelper.Setup(t)

	for name, tc := range map[string]struct {
		ws      int64
		channel string
	}{
		"email":                    {fixtures.AcmeID, eligibility.ChannelEmail},
		"email under confirmation": {fixtures.LumonID, eligibility.ChannelEmail},
		"sms":                      {fixtures.TyrellID, eligibility.ChannelSMS},
	} {
		d, err := eligibility.Check(context.Background(), env.DB.Scoped(tc.ws), tc.channel, "   ", eligibility.SourceBroadcasts)

		require.NoError(t, err, name)
		assert.Equal(t, eligibility.Decision{Eligible: true}, d, name)
	}
}

func TestCheckFailsClosedOnStoreErrorAndUnknownWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail, "a@example.com", eligibility.SourceBroadcasts)
	require.ErrorContains(t, err, "eligibility check")
	require.ErrorIs(t, err, context.Canceled, "the store's error is wrapped, not replaced")

	_, err = eligibility.Check(context.Background(), env.DB.Scoped(987654), eligibility.ChannelEmail, "a@example.com", eligibility.SourceBroadcasts)
	assert.ErrorContains(t, err, "workspace 987654 not found")
}

// GloballyOptedOut matches suppressed and unsubscribed-from-everything contacts,
// but not a per-source opt-out and not a clean contact.
func TestGloballyOptedOut(t *testing.T) {
	env := testhelper.Setup(t)

	emails, err := env.DB.Scoped(fixtures.TyrellID).Contact().Query().
		Where(eligibility.GloballyOptedOut(eligibility.ChannelEmail)).
		Select("email").Strings(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		fixtures.TyrellMatrixSuppressedEmail, fixtures.TyrellMatrixUnsubEverythingEmail,
		fixtures.TyrellMatrixConfirmedSuppressedEmail,
	}, emails)
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
