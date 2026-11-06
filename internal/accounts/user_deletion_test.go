package accounts_test

import (
	"context"
	"sync"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
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

// Two co-Owners deleting themselves at the same moment serialize on the Workspace's Owner
// rows (ADR 0050): exactly one is deleted, the other is refused as the last Owner.
func TestTwoCoOwnersDeletingThemselvesAtOnceDeleteExactlyOne(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	client := entclient.New(sqlDB)
	bus := events.New(sqlDB, entclient.NewTx)
	acc := accounts.New(client, bus, testhelper.Passwords(), testhelper.ConsentRecords(client, bus))

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, userID := range []int64{fixtures.InitechOwnerOlgaID, fixtures.InitechCoOwnerOwenID} {
		wg.Go(func() { _, errs[i] = acc.DeleteUser(t.Context(), userID, nil) })
	}
	wg.Wait()

	refused := lo.Filter(errs, func(err error, _ int) bool { return err != nil })
	require.Len(t, refused, 1)
	require.ErrorIs(t, refused[0], accounts.ErrSoleOwner)
	var sole *accounts.DeletionRefusedError
	require.ErrorAs(t, refused[0], &sole)
	assert.Equal(t, fixtures.InitechName, sole.Workspaces[0].Name)
	owners, err := client.Scoped(fixtures.InitechID).Membership().Query().Where(membership.RoleEQ(membership.RoleOwner)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, owners)
}
