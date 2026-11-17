package invitations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/internal/domain/identity/invitations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestAcceptanceCannotReplaceAnEarlierAcceptance(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	at := time.Now()
	require.NoError(t, invitations.New(env.DB, env.Bus, func() time.Time { return at }).MarkAcceptedIn(t.Context(), s, fixtures.InvitationAcmePendingID))
	later := invitations.New(env.DB, env.Bus, func() time.Time { return at.Add(time.Hour) })

	err := later.MarkAcceptedIn(t.Context(), s, fixtures.InvitationAcmePendingID)

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	stored, err := s.Invitation().Get(t.Context(), fixtures.InvitationAcmePendingID)
	require.NoError(t, err)
	require.NotNil(t, stored.AcceptedAt)
	assert.WithinDuration(t, at, *stored.AcceptedAt, time.Microsecond)
}

func TestReinvitingAnAcceptedAddressPreservesItsAcceptance(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	at := time.Now()
	invites := invitations.New(env.DB, env.Bus, func() time.Time { return at })
	require.NoError(t, invites.MarkAcceptedIn(t.Context(), s, fixtures.InvitationAcmePendingID))

	reissued, err := invites.Invite(t.Context(), s, events.Actor{Kind: events.ActorUser}, invitations.InviteInput{
		Email: fixtures.InvitationAcmePendingEmail, Role: invitation.RoleMember, InvitedBy: fixtures.OwnerJohnID,
	})

	require.NoError(t, err)
	issued, token := reissued.Invitation, reissued.Token
	assert.NotEqual(t, int64(fixtures.InvitationAcmePendingID), issued.ID)
	assert.Nil(t, issued.AcceptedAt)
	accepted, err := s.Invitation().Get(t.Context(), fixtures.InvitationAcmePendingID)
	require.NoError(t, err)
	require.NotNil(t, accepted.AcceptedAt)
	assert.WithinDuration(t, at, *accepted.AcceptedAt, time.Microsecond)
	pending, err := invites.PendingInvitation(t.Context(), token)
	require.NoError(t, err)
	assert.Equal(t, issued.ID, pending.ID)
}

func TestAnAcceptedInvitationCannotBeRevoked(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	at := time.Now()
	invites := invitations.New(env.DB, env.Bus, func() time.Time { return at })
	require.NoError(t, invites.MarkAcceptedIn(t.Context(), s, fixtures.InvitationAcmePendingID))

	err := invites.RevokeInvitation(t.Context(), s, events.Actor{Kind: events.ActorUser}, fixtures.InvitationAcmePendingID)

	require.ErrorIs(t, err, invitations.ErrInvitationNotFound)
	accepted, err := s.Invitation().Get(t.Context(), fixtures.InvitationAcmePendingID)
	require.NoError(t, err)
	require.NotNil(t, accepted.AcceptedAt)
	assert.WithinDuration(t, at, *accepted.AcceptedAt, time.Microsecond)
}

func TestInviteOwnsTokenHashAndExpiry(t *testing.T) {
	env := testhelper.Setup(t)
	invites := invitations.New(env.DB, env.Bus, time.Now)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	actor := events.Actor{Kind: events.ActorUser}
	in := invitations.InviteInput{Email: "newbie@acme.test", Role: invitation.RoleAdmin, InvitedBy: fixtures.OwnerJohnID}

	t.Run("issues a random token stored only as its hash with a 7-day expiry", func(t *testing.T) {
		before := time.Now()
		issued, err := invites.Invite(ctx, s, actor, in)
		require.NoError(t, err)
		inv, token := issued.Invitation, issued.Token

		assert.Regexp(t, `^[A-Za-z0-9_-]+$`, token, "it travels in a link as it is")
		assert.Equal(t, credentials.HashSecret(token), inv.TokenHash)
		assert.Len(t, inv.TokenHash, 64, "sha-256 hex")
		assert.NotContains(t, inv.TokenHash, token)
		assert.WithinDuration(t, before.Add(invitations.InviteTokenTTL), inv.ExpiresAt, 5*time.Second)
		assert.Equal(t, 7*24*time.Hour, invitations.InviteTokenTTL)

		pending, err := invites.PendingInvitation(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, inv.ID, pending.ID)
	})

	t.Run("reissuing rotates the token and invalidates the old link", func(t *testing.T) {
		first, err := invites.Invite(ctx, s, actor, in)
		require.NoError(t, err)
		second, err := invites.Invite(ctx, s, actor, in)
		require.NoError(t, err)

		assert.Equal(t, first.Invitation.ID, second.Invitation.ID)
		assert.NotEqual(t, first.Token, second.Token)
		_, err = invites.PendingInvitation(ctx, first.Token)
		assert.Error(t, err)
	})

	t.Run("reissues a fixture invitation without touching its neighbours", func(t *testing.T) {
		issued, err := invites.Invite(ctx, s, actor, invitations.InviteInput{Email: "invited@acme.test", Role: invitation.RoleMember, InvitedBy: fixtures.OwnerJohnID})
		require.NoError(t, err)
		_, err = invites.PendingInvitation(ctx, "inv_fixture_token_acme")
		require.Error(t, err, "the fixture token was rotated away")
		_, err = invites.PendingInvitation(ctx, issued.Token)
		assert.NoError(t, err)
	})

	t.Run("an existing member is refused", func(t *testing.T) {
		_, err := invites.Invite(ctx, s, actor, invitations.InviteInput{Email: "  " + fixtures.OwnerJohnEmail + " ", Role: invitation.RoleMember, InvitedBy: fixtures.OwnerJohnID})
		require.ErrorIs(t, err, invitations.ErrAlreadyMember)
	})

	t.Run("a blank email is refused", func(t *testing.T) {
		_, err := invites.Invite(ctx, s, actor, invitations.InviteInput{Email: "   ", Role: invitation.RoleMember, InvitedBy: fixtures.OwnerJohnID})
		require.ErrorIs(t, err, invitations.ErrEmailEmpty)
	})
}
