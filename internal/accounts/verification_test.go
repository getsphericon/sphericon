package accounts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The verification link is single use even when two submissions race past the
// handler's own check: the write itself only lands on a still-Unverified User.
func TestVerifyEmailLandsOnlyOnAnUnverifiedUser(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus)
	u, err := acc.Register(t.Context(), "Grace", "grace@example.com", "registered-hash")
	require.NoError(t, err)

	_, err = acc.VerifyEmail(t.Context(), u.ID, new("hash-one"))
	require.NoError(t, err)
	_, err = acc.VerifyEmail(t.Context(), u.ID, new("hash-two"))
	require.ErrorIs(t, err, accounts.ErrAlreadyVerified)

	stored, err := acc.User(t.Context(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, "hash-one", stored.PasswordHash, "the second submission overwrote nothing")
	assert.EqualValues(t, 1, stored.SessionEpoch, "verification ends earlier sessions once")
}

// A Verified User is attached only with their own session, decided inside the
// transaction: whatever the caller believed about the address, no session is refused.
func TestAcceptInvitationOfAVerifiedUserNeedsTheirSession(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus)
	inv, _, err := acc.Invite(t.Context(), env.DB.Scoped(fixtures.AcmeID), events.Actor{Kind: events.ActorUser},
		accounts.InviteInput{Email: fixtures.OutsiderOscarEmail, Role: invitation.RoleMember, InvitedBy: fixtures.OwnerJohnID})
	require.NoError(t, err)

	require.ErrorIs(t, acc.AcceptInvitation(t.Context(), inv, "", "", ""), accounts.ErrSignInRequired)
	require.ErrorIs(t, acc.AcceptInvitation(t.Context(), inv, "", "", fixtures.OwnerJaneEmail), accounts.ErrWrongAccount)
	require.NoError(t, acc.AcceptInvitation(t.Context(), inv, "", "", fixtures.OutsiderOscarEmail))
}
