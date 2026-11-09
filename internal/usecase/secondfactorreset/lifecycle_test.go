package secondfactorreset_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/recoverycode"
	"github.com/getsphericon/sphericon/ent/secondfactor"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/secondfactorreset"
)

func TestLifecycleResetCancelsPendingEnrollment(t *testing.T) {
	env, _ := setup(t)
	_, err := env.SecondFactor.StartEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	before, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)

	err = env.SecondFactorReset.ResetByStaff(t.Context(), fixtures.OwnerJohnEmail, "cli")

	require.NoError(t, err)
	status, err := env.SecondFactor.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.False(t, status.Pending)
	assert.False(t, status.Enabled)
	after, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch+1, after.SessionEpoch)
}

func TestLifecycleResetNeverReusesAnEnrollmentIdentity(t *testing.T) {
	env, c := setup(t)
	first, err := env.SecondFactor.StartEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	err = env.SecondFactorReset.ResetByStaff(t.Context(), fixtures.OwnerJohnEmail, "cli")
	require.NoError(t, err)
	second, err := env.SecondFactor.StartEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	assert.Greater(t, second.ID, first.ID)
	code, err := totp.GenerateCode(second.Secret, c.now())
	require.NoError(t, err)

	_, err = env.SecondFactor.ConfirmEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword}, secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code})

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
}

type factorRace struct {
	client  *ent.Client
	bus     *events.Bus
	factors *secondfactors.Module
	reset   *secondfactorreset.UseCase
	now     time.Time
}

func newFactorRace(t *testing.T) factorRace {
	t.Helper()
	pool := testhelper.RealDB(t)
	pool.SetMaxOpenConns(4)
	client := entclient.New(pool)
	now := time.Now()
	clock := func() time.Time { return now }
	bus := events.New(pool, entclient.NewTx, clock)
	bus.UseJobs(testhelper.AcceptingQueue{})
	return factorRace{client: client, bus: bus, now: now,
		factors: secondfactors.New(client, bus, testhelper.Cipher(t), testhelper.ProvenUsers{}, clock),
		reset:   secondfactorreset.New(bus)}
}

// pause holds the first write that ends the User's sessions (the bump of the session epoch, the
// first write of a confirmation or a regeneration, before the factor row), after its operation
// loaded and checked the User and the factor and before the write takes any lock, until unblock.
func (r factorRace) pause(t *testing.T) (paused <-chan struct{}, unblock func()) {
	t.Helper()
	ready, resume := make(chan struct{}, 1), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	t.Cleanup(release)
	// Only the first match pauses; a later one (the competing operation) runs on: sync.Once would
	// make it wait for the paused one.
	var fired atomic.Bool
	hook := func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if u, ok := mutation.(*ent.UserMutation); ok {
				if epoch, adds := u.AddedSessionEpoch(); adds && epoch > 0 {
					if fired.CompareAndSwap(false, true) {
						ready <- struct{}{}
						select {
						case <-resume:
						case <-ctx.Done():
						}
					}
				}
			}
			return next.Mutate(ctx, mutation)
		})
	}
	r.client.Use(hook)
	r.bus.Use(hook)
	return ready, release
}

func TestLifecycleConfirmationCannotUndoAReset(t *testing.T) {
	r := newFactorRace(t)
	first, err := r.factors.StartEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	ready, release := r.pause(t)
	code, err := totp.GenerateCode(first.Secret, r.now)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		_, err := r.factors.ConfirmEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword}, secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code})
		result <- err
	}()
	testhelper.Receive(t, ready, "confirmation checked the secret before reset")
	err = r.reset.ResetByStaff(t.Context(), fixtures.OwnerJohnEmail, "cli")
	require.NoError(t, err)
	release()

	stale := testhelper.Receive(t, result, "confirmation after reset finished")

	require.ErrorIs(t, stale, lifecycle.ErrInvalidTransition)
	status, err := r.factors.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.False(t, status.Enabled)
	assert.False(t, status.Pending)
	assert.Zero(t, status.RecoveryCodesRemaining)
}

func TestLifecycleRegenerationCannotRecreateCodesAfterReset(t *testing.T) {
	r := newFactorRace(t)
	ready, release := r.pause(t)
	result := make(chan error, 1)
	go func() {
		_, err := r.factors.RegenerateRecoveryCodes(t.Context(), r.client.User.GetX(t.Context(), fixtures.SecondFactorSamID), users.IdentityProof{Password: fixtures.SecondFactorSamPassword})
		result <- err
	}()
	testhelper.Receive(t, ready, "regeneration checked the active factor")
	err := r.reset.ResetByStaff(t.Context(), fixtures.SecondFactorSamEmail, "cli")
	require.NoError(t, err)
	release()

	stale := testhelper.Receive(t, result, "regeneration after reset finished")

	require.ErrorIs(t, stale, lifecycle.ErrInvalidTransition)
	f, err := r.client.SecondFactor.Query().Where(secondfactor.UserID(fixtures.SecondFactorSamID)).Only(t.Context())
	require.NoError(t, err)
	assert.Empty(t, f.SecretEncrypted)
	codes, err := r.client.RecoveryCode.Query().Where(recoverycode.UserID(fixtures.SecondFactorSamID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, codes)
}
