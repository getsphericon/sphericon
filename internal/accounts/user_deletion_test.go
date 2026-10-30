package accounts_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A deletion whose proof was read for a User with (or without) a password is refused inside
// the transaction once the User's password was removed (or set) in between (ADR 0050): the
// User stays, and the second-factor code is not spent.
func TestDeleteUserRefusesAProofThatNoLongerFitsTheUser(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	ctx := context.Background()
	u, err := env.DB.User.Get(ctx, fixtures.MemberMaryID)
	require.NoError(t, err)

	spent := false
	confirm := func(context.Context, *ent.Client, events.Publisher, *ent.User) error {
		spent = true
		return nil
	}
	_, err = acc.DeleteUser(ctx, u.ID, &accounts.DeleteProof{HadPassword: u.PasswordHash == "", Confirm: confirm})

	require.ErrorIs(t, err, accounts.ErrProofStale)
	assert.False(t, spent)
	_, err = env.DB.User.Get(ctx, u.ID)
	require.NoError(t, err, "the User is not deleted")
}

// A proof that still fits the User spends its code inside the transaction and the User goes.
func TestDeleteUserSpendsTheCodeOfAProofThatStillFits(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	ctx := context.Background()
	u, err := env.DB.User.Get(ctx, fixtures.MemberMaryID)
	require.NoError(t, err)

	spent := 0
	confirm := func(context.Context, *ent.Client, events.Publisher, *ent.User) error {
		spent++
		return nil
	}
	deleted, err := acc.DeleteUser(ctx, u.ID, &accounts.DeleteProof{HadPassword: u.PasswordHash != "", Confirm: confirm})

	require.NoError(t, err)
	assert.Equal(t, u.ID, deleted.ID)
	assert.Equal(t, 1, spent)
	_, err = env.DB.User.Get(ctx, u.ID)
	assert.True(t, ent.IsNotFound(err))
}
