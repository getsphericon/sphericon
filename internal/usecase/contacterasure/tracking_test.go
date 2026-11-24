package contacterasure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/engagementtrackingconsent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

// recordTracking records a member's observation of an Acme Destination.
func recordTracking(t *testing.T, env *testhelper.TestEnv, channel channels.Channel, destination string, state engagementtracking.State) {
	t.Helper()
	_, err := testhelper.Module[*engagementtracking.Module](env).Record(t.Context(), env.DB.Scoped(fixtures.AcmeID), engagementtracking.RecordInput{
		Channel: channel, Destination: destination, State: state, Provenance: engagementtracking.Manual,
	})
	require.NoError(t, err)
}

// An Erasure keeps the tracking refusal of the Contact's number by the number's hash, as it keeps
// its opt-out (ADR 0120, ADR 0021): the number leaves only its hash.
func TestAnErasureKeepsTheTrackingRefusalOfANumberByItsHash(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	recordTracking(t, env, channels.SMS, fixtures.ContactTexterPhone, engagementtracking.Denied)

	require.NoError(t, erase(t, env, fixtures.AcmeID, contacterasure.Identifier{ContactID: fixtures.ContactTexterID}))

	named, err := env.DB.EngagementTrackingConsent.Query().Where(engagementtrackingconsent.Destination(fixtures.ContactTexterPhone)).Exist(ctx)
	require.NoError(t, err)
	kept, err := env.DB.EngagementTrackingConsent.Query().Where(
		engagementtrackingconsent.ChannelEQ(channels.SMS), engagementtrackingconsent.Destination(sha256Hex(fixtures.ContactTexterPhone)),
	).Only(ctx)
	require.NoError(t, err)
	assert.False(t, named, "no consent names the number")
	assert.Equal(t, engagementtracking.Denied, kept.State)
}

// An Erasure deletes the tracking grant of the Contact's Telegram chat, as it deletes a
// Confirmation: the user id follows the default again.
func TestAnErasureDropsTheTrackingGrantOfAChat(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	recordTracking(t, env, channels.Telegram, fixtures.BindingLiamDestination, engagementtracking.Granted)

	require.NoError(t, erase(t, env, fixtures.AcmeID, contacterasure.Identifier{ContactID: fixtures.ContactLiamID}))

	after, err := testhelper.Module[*engagementtracking.Module](env).Of(ctx, env.DB.Scoped(fixtures.AcmeID), channels.Telegram, fixtures.BindingLiamDestination)
	require.NoError(t, err)
	assert.Equal(t, engagementtracking.ReadingDefault, after.State)
}
