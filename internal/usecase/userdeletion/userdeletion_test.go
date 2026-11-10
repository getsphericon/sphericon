package userdeletion_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/userdeletion"
)

// samRequest is Sam's own deletion, proven with the password and a Recovery code.
func samRequest() userdeletion.Request {
	return userdeletion.Request{
		UserID:   fixtures.SecondFactorSamID,
		Email:    fixtures.SecondFactorSamEmail,
		Password: fixtures.SecondFactorSamPassword,
		Code:     fixtures.SecondFactorSamRecoveryCode,
	}
}

// A deletion whose proof was read for a User with a password is refused inside the transaction
// once the User's password was removed in between (ADR 0050): the User stays, and the
// Second factor code is not spent.
func TestDeleteRefusesAProofThatNoLongerFitsTheUser(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	before, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	// The password goes right after the deletion read the User it proves the password of.
	var removed atomic.Bool
	env.DB.User.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
			v, err := next.Query(ctx, q)
			if err == nil && removed.CompareAndSwap(false, true) {
				err = env.FeedDB.User.UpdateOneID(fixtures.SecondFactorSamID).SetPasswordHash("").Exec(ctx)
			}
			return v, err
		})
	}))

	err = env.UserDeletion.Delete(ctx, samRequest())

	require.ErrorIs(t, err, users.ErrProofStale)
	_, err = env.DB.User.Get(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err, "the User is not deleted")
	after, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Equal(t, before.RecoveryCodesRemaining, after.RecoveryCodesRemaining, "the code is not spent")
}

// A proof that still fits the User spends its code inside the transaction and the User goes.
func TestDeleteSpendsTheCodeOfAProofThatStillFits(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	err := env.UserDeletion.Delete(ctx, samRequest())

	require.NoError(t, err)
	_, err = env.DB.User.Get(ctx, fixtures.SecondFactorSamID)
	assert.True(t, ent.IsNotFound(err))
}

// A wrong code refuses the deletion as a failed proof and deletes nothing.
func TestDeleteRefusesAWrongCode(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	req := samRequest()
	req.Code = fixtures.SecondFactorSamSpentRecoveryCode

	err := env.UserDeletion.Delete(ctx, req)

	require.ErrorIs(t, err, secondfactors.ErrDeletionCodeInvalid)
	_, err = env.DB.User.Get(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err, "the User is not deleted")
}

// Two co-Owners deleting themselves at the same moment serialize on the Workspace's Owner
// rows (ADR 0050): exactly one is deleted, the other is refused as the last Owner.
func TestTwoCoOwnersDeletingThemselvesAtOnceDeleteExactlyOne(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	client := entclient.Core.New(sqlDB)
	bus := events.New(sqlDB, entclient.Core.NewTx, time.Now)
	bus.UseJobs(testhelper.AcceptingQueue{})
	usersModule := users.New(client, bus, testhelper.Passwords(), testhelper.ConsentRecords(client, bus), time.Now)
	factors := secondfactors.New(client, bus, testhelper.Cipher(t), nil, time.Now)
	budget := throttle.NewAttempts(client, time.Now).For(throttle.KindLogin)
	// Both prove a password, so no deletion link is checked.
	deletion := userdeletion.New(bus, usersModule, nil, factors, budget, testhelper.Passwords())

	owners := []userdeletion.Request{
		{UserID: fixtures.InitechOwnerOlgaID, Email: fixtures.InitechOwnerOlgaEmail, Password: fixtures.InitechOwnerOlgaPassword},
		{UserID: fixtures.InitechCoOwnerOwenID, Email: fixtures.InitechCoOwnerOwenEmail, Password: fixtures.InitechCoOwnerOwenPassword},
	}
	errs := make([]error, len(owners))
	var wg sync.WaitGroup
	for i, req := range owners {
		wg.Go(func() { errs[i] = deletion.Delete(t.Context(), req) })
	}
	wg.Wait()

	refused := lo.Filter(errs, func(err error, _ int) bool { return err != nil })
	require.Len(t, refused, 1)
	require.ErrorIs(t, refused[0], users.ErrSoleOwner)
	var sole *users.DeletionRefusedError
	require.ErrorAs(t, refused[0], &sole)
	assert.Equal(t, fixtures.InitechName, sole.Workspaces[0].Name)
	left, err := client.Scoped(fixtures.InitechID).Membership().Query().Where(membership.RoleEQ(membership.RoleOwner)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, left)
}
