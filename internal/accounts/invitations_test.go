package accounts_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestInviteOwnsTokenHashAndExpiry(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	actor := events.Actor{Kind: events.ActorUser}
	in := accounts.InviteInput{Email: "newbie@acme.test", Role: invitation.RoleAdmin, InvitedBy: fixtures.OwnerJohnID}

	t.Run("issues a random token stored only as its hash with a 7-day expiry", func(t *testing.T) {
		before := time.Now()
		inv, token, err := acc.Invite(ctx, s, actor, in)
		require.NoError(t, err)

		assert.Len(t, token, 48)
		assert.Equal(t, accounts.HashInviteToken(token), inv.TokenHash)
		assert.Len(t, inv.TokenHash, 64, "sha-256 hex")
		assert.NotContains(t, inv.TokenHash, token)
		assert.WithinDuration(t, before.Add(accounts.InviteTokenTTL), inv.ExpiresAt, 5*time.Second)
		assert.Equal(t, 7*24*time.Hour, accounts.InviteTokenTTL)

		pending, err := acc.PendingInvitation(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, inv.ID, pending.ID)
	})

	t.Run("reissuing rotates the token and invalidates the old link", func(t *testing.T) {
		first, oldToken, err := acc.Invite(ctx, s, actor, in)
		require.NoError(t, err)
		second, newToken, err := acc.Invite(ctx, s, actor, in)
		require.NoError(t, err)

		assert.Equal(t, first.ID, second.ID)
		assert.NotEqual(t, oldToken, newToken)
		_, err = acc.PendingInvitation(ctx, oldToken)
		assert.Error(t, err)
	})

	t.Run("reissues a fixture invitation without touching its neighbours", func(t *testing.T) {
		_, token, err := acc.Invite(ctx, s, actor, accounts.InviteInput{Email: "invited@acme.test", Role: invitation.RoleMember, InvitedBy: fixtures.OwnerJohnID})
		require.NoError(t, err)
		_, err = acc.PendingInvitation(ctx, "inv_fixture_token_acme")
		require.Error(t, err, "the fixture token was rotated away")
		_, err = acc.PendingInvitation(ctx, token)
		assert.NoError(t, err)
	})

	t.Run("an existing member is refused", func(t *testing.T) {
		_, _, err := acc.Invite(ctx, s, actor, accounts.InviteInput{Email: "  " + fixtures.OwnerJohnEmail + " ", Role: invitation.RoleMember, InvitedBy: fixtures.OwnerJohnID})
		require.ErrorIs(t, err, accounts.ErrAlreadyMember)
	})

	t.Run("a blank email is refused", func(t *testing.T) {
		_, _, err := acc.Invite(ctx, s, actor, accounts.InviteInput{Email: "   ", Role: invitation.RoleMember, InvitedBy: fixtures.OwnerJohnID})
		require.ErrorIs(t, err, accounts.ErrEmailEmpty)
	})
}
