package secondfactors_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/secondfactor"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A pending enrollment is not a Second factor: its secret verifies no second step, and the steps
// that need an active factor (regenerating the Recovery codes, disabling) have nothing to act on.
func TestAPendingEnrollmentCannotVerifyRegenerateOrDisable(t *testing.T) {
	env, c := setup(t)
	john := env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID)
	proof := users.IdentityProof{Password: fixtures.OwnerJohnPassword}
	enrollment, err := env.SecondFactor.StartEnrollment(t.Context(), john, proof)
	require.NoError(t, err)
	code := enrollmentCode(t, enrollment.Secret, c.now())

	_, verified := verify(t, env, fixtures.OwnerJohnID, code)
	_, regenerated := env.SecondFactor.RegenerateRecoveryCodes(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), proof)
	_, disabled := env.SecondFactor.Disable(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), proof, code)

	require.ErrorIs(t, verified, secondfactors.ErrNotActive)
	require.ErrorIs(t, regenerated, secondfactors.ErrNotActive)
	require.ErrorIs(t, disabled, secondfactors.ErrNotActive)
	status, err := env.SecondFactor.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, secondfactors.Status{Pending: true}, status)
}

type factorRace struct {
	client  *ent.Client
	bus     *events.Bus
	factors *secondfactors.Module
	now     time.Time
}

func newFactorRace(t *testing.T) factorRace {
	t.Helper()
	pool := testhelper.RealDB(t)
	pool.SetMaxOpenConns(4)
	client := entclient.Core.New(pool)
	now := time.Now()
	clock := func() time.Time { return now }
	bus := events.New(pool, entclient.Core.NewTx, clock)
	bus.UseJobs(testhelper.AcceptingQueue{})
	return factorRace{client: client, bus: bus, now: now,
		factors: secondfactors.New(client, bus, testhelper.Cipher(t), testhelper.ProvenUsers{}, clock)}
}

// pause holds the first write matches selects, after its operation loaded and checked the User
// and the factor and before the write takes any lock, until unblock.
func (r factorRace) pause(t *testing.T, matches func(ent.Mutation) bool) (paused <-chan struct{}, unblock func()) {
	t.Helper()
	ready, resume := make(chan struct{}, 1), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	t.Cleanup(release)
	// Only the first match pauses; a later one (the competing operation) runs on: sync.Once would
	// make it wait for the paused one.
	var fired atomic.Bool
	hook := func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if matches(mutation) {
				if fired.CompareAndSwap(false, true) {
					ready <- struct{}{}
					select {
					case <-resume:
					case <-ctx.Done():
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

// startsEnrollment is the write of a Start: the factor's new secret.
func startsEnrollment(m ent.Mutation) bool {
	f, ok := m.(*ent.SecondFactorMutation)
	if !ok {
		return false
	}
	_, set := f.SecretEncrypted()
	return set
}

// acceptsStep is the factor's write of a matched TOTP step (a confirmation or a second step).
func acceptsStep(m ent.Mutation) bool {
	f, ok := m.(*ent.SecondFactorMutation)
	if !ok {
		return false
	}
	step, set := f.LastStep()
	return set && step > 0
}

// endsSessions is the first write of a change that ends the User's sessions: the bump of the
// User's session epoch, before the factor row.
func endsSessions(m ent.Mutation) bool {
	u, ok := m.(*ent.UserMutation)
	if !ok {
		return false
	}
	epoch, adds := u.AddedSessionEpoch()
	return adds && epoch > 0
}

func enrollmentCode(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, at)
	require.NoError(t, err)
	return code
}

func TestLifecycleAnOlderStartCannotReplaceAConfirmedFactor(t *testing.T) {
	r := newFactorRace(t)
	first, err := r.factors.StartEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	ready, release := r.pause(t, startsEnrollment)
	result := make(chan error, 1)
	go func() {
		_, err := r.factors.StartEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
		result <- err
	}()
	testhelper.Receive(t, ready, "replacement enrollment reached its conditional write")
	code := enrollmentCode(t, first.Secret, r.now)
	_, err = r.factors.ConfirmEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword}, secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code})
	require.NoError(t, err)
	release()

	stale := testhelper.Receive(t, result, "older Start finished")

	require.ErrorIs(t, stale, lifecycle.ErrInvalidTransition)
	status, err := r.factors.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.True(t, status.Enabled)
	assert.False(t, status.Pending)
}

func TestLifecycleConfirmationCannotActivateAReplacementSecret(t *testing.T) {
	r := newFactorRace(t)
	first, err := r.factors.StartEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	ready, release := r.pause(t, endsSessions)
	code := enrollmentCode(t, first.Secret, r.now)
	result := make(chan error, 1)
	go func() {
		_, err := r.factors.ConfirmEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword}, secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code})
		result <- err
	}()
	testhelper.Receive(t, ready, "confirmation checked the original secret")
	_, err = r.factors.StartEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	release()

	stale := testhelper.Receive(t, result, "old confirmation finished")

	require.ErrorIs(t, stale, lifecycle.ErrInvalidTransition)
	status, err := r.factors.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.False(t, status.Enabled)
	assert.True(t, status.Pending)
	assert.Zero(t, status.RecoveryCodesRemaining)
}

func TestLifecycleConfirmationNamesTheEnrollmentNotJustAValidCode(t *testing.T) {
	env, c := setup(t)
	before, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	first, err := env.SecondFactor.StartEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	second, err := env.SecondFactor.StartEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	assert.Greater(t, second.ID, first.ID)
	pendingUser, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch, pendingUser.SessionEpoch, "opening a QR does not end sessions")
	code := enrollmentCode(t, second.Secret, c.now())

	_, err = env.SecondFactor.ConfirmEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword}, secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code})

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	status, err := env.SecondFactor.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.True(t, status.Pending)
	assert.Zero(t, status.RecoveryCodesRemaining)
	issued, err := env.SecondFactor.ConfirmEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword}, secondfactors.ConfirmInput{EnrollmentID: second.ID, Code: code})
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch+1, issued.User.SessionEpoch)
	assert.NotEmpty(t, issued.RecoveryCodes)
}

func TestLifecycleAnAuditFailureRollsBackConfirmationAndCodes(t *testing.T) {
	r := newFactorRace(t)
	enrollment, err := r.factors.StartEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	before, err := r.client.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	factorBefore, err := r.client.SecondFactor.Query().Where(secondfactor.UserID(fixtures.OwnerJohnID)).Only(t.Context())
	require.NoError(t, err)
	r.bus.UseJobs(nil)
	r.bus.UseConsumers(events.Consumer{
		Wants: func(_ events.Envelope, event events.DomainEvent) bool {
			return event.EventName() == events.NameAuditEntry
		},
		Job: func(envelope events.Envelope, _ events.DomainEvent) river.JobArgs {
			return factorAuditArgs{events.EventJob{Envelope: envelope}}
		},
	})
	code := enrollmentCode(t, enrollment.Secret, r.now)

	_, err = r.factors.ConfirmEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword}, secondfactors.ConfirmInput{EnrollmentID: enrollment.ID, Code: code})

	require.ErrorIs(t, err, events.ErrNoJobQueue)
	after, err := r.client.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	factorAfter, err := r.client.SecondFactor.Query().Where(secondfactor.UserID(fixtures.OwnerJohnID)).Only(t.Context())
	require.NoError(t, err)
	assert.Nil(t, factorAfter.ConfirmedAt)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch)
	assert.Equal(t, factorBefore.LastStep, factorAfter.LastStep)
	codes, err := r.client.User.QueryRecoveryCodes(after).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, codes)
}

type factorAuditArgs struct{ events.EventJob }

func (factorAuditArgs) Kind() string { return "test_factor_audit" }

func TestLifecycleTOTPAndRecoveryCodeCannotBothSpendOneChallenge(t *testing.T) {
	r := newFactorRace(t)
	u, err := r.client.User.Get(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	challenged, err := r.factors.ChallengeBinding(t.Context(), u)
	require.NoError(t, err)
	ready, release := r.pause(t, acceptsStep)
	totpResult, recoveryResult := make(chan error, 1), make(chan error, 1)
	code := enrollmentCode(t, fixtures.SamSecondFactorTotpSecret, r.now)
	go func() {
		_, err := r.factors.Verify(t.Context(), pending(fixtures.SecondFactorSamID, challenged), code)
		totpResult <- err
	}()
	testhelper.Receive(t, ready, "TOTP verification holds the User lock")
	go func() {
		_, err := r.factors.Verify(t.Context(), pending(fixtures.SecondFactorSamID, challenged), fixtures.SecondFactorSamRecoveryCode)
		recoveryResult <- err
	}()
	release()

	first := testhelper.Receive(t, totpResult, "TOTP verification finished")
	second := testhelper.Receive(t, recoveryResult, "recovery verification finished")

	require.NoError(t, first)
	require.ErrorIs(t, second, secondfactors.ErrBindingMoved)
	status, err := r.factors.Status(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Equal(t, 1, status.RecoveryCodesRemaining, "the losing challenge does not spend a recovery code")
}
