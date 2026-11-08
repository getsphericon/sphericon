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
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/secondfactor"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSecondFactorUnconfirmedSecretsCannotVerifyOrRegenerate(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secret := rapid.SampledFrom([]string{"", "sealed pending secret"}).Draw(t, "unconfirmed factor")
		trigger := rapid.SampledFrom([]secondfactors.Trigger{secondfactors.FactorVerify, secondfactors.FactorRegenerate, secondfactors.FactorDisable}).Draw(t, "active-only operation")
		before := &ent.SecondFactor{SecretEncrypted: secret}
		stored := secondfactors.State("")
		machine := secondfactors.Machine(before, secondfactors.Change{Trigger: trigger, Step: 1}, func(_ context.Context, to secondfactors.State) (bool, error) {
			stored = to
			return true, nil
		})

		err := machine.FireCtx(t.Context(), trigger)

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Empty(t, stored)
	})
}

func TestSecondFactorConfirmationAlwaysRequiresANewerStep(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		previous := rapid.Int64Range(1, 1000000).Draw(t, "accepted step")
		step := rapid.Int64Range(0, previous).Draw(t, "replayed or older step")
		before := &ent.SecondFactor{SecretEncrypted: "sealed pending secret", LastStep: previous}
		stored := secondfactors.State("")
		machine := secondfactors.Machine(before, secondfactors.Change{Trigger: secondfactors.FactorConfirm, Step: step}, func(_ context.Context, to secondfactors.State) (bool, error) {
			stored = to
			return true, nil
		})

		err := machine.FireCtx(t.Context(), secondfactors.FactorConfirm)

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Empty(t, stored)
	})
}

func TestSecondFactorInternalTransitionsCannotSkipTheirConditionalWrite(t *testing.T) {
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		before *ent.SecondFactor
		change secondfactors.Change
	}{
		{"replace pending", &ent.SecondFactor{SecretEncrypted: "sealed"}, secondfactors.Change{Trigger: secondfactors.FactorStart, SealedSecret: "replacement"}},
		{"verify totp", &ent.SecondFactor{SecretEncrypted: "sealed", ConfirmedAt: &now}, secondfactors.Change{Trigger: secondfactors.FactorVerify, Step: 1}},
		{"verify recovery", &ent.SecondFactor{SecretEncrypted: "sealed", ConfirmedAt: &now}, secondfactors.Change{Trigger: secondfactors.FactorVerify}},
		{"regenerate", &ent.SecondFactor{SecretEncrypted: "sealed", ConfirmedAt: &now}, secondfactors.Change{Trigger: secondfactors.FactorRegenerate}},
		{"reset passkeys", nil, secondfactors.Change{Trigger: secondfactors.FactorReset}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := secondfactors.Machine(tc.before, tc.change, func(context.Context, secondfactors.State) (bool, error) { return false, nil })

			err := machine.FireCtx(t.Context(), tc.change.Trigger)

			require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		})
	}
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
	client := entclient.New(pool)
	now := time.Now()
	clock := func() time.Time { return now }
	bus := events.New(pool, entclient.NewTx, clock)
	bus.UseJobs(testhelper.AcceptingQueue{})
	return factorRace{client: client, bus: bus, now: now,
		factors: secondfactors.New(client, bus, testhelper.Cipher(t), nil, clock)}
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
	first, err := r.factors.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	ready, release := r.pause(t, startsEnrollment)
	result := make(chan error, 1)
	go func() {
		_, err := r.factors.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
		result <- err
	}()
	testhelper.Receive(t, ready, "replacement enrollment reached its conditional write")
	code := enrollmentCode(t, first.Secret, r.now)
	_, err = r.factors.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code})
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
	first, err := r.factors.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	ready, release := r.pause(t, endsSessions)
	code := enrollmentCode(t, first.Secret, r.now)
	result := make(chan error, 1)
	go func() {
		_, err := r.factors.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code})
		result <- err
	}()
	testhelper.Receive(t, ready, "confirmation checked the original secret")
	_, err = r.factors.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
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
	first, err := env.SecondFactor.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	second, err := env.SecondFactor.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Greater(t, second.ID, first.ID)
	pendingUser, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch, pendingUser.SessionEpoch, "opening a QR does not end sessions")
	code := enrollmentCode(t, second.Secret, c.now())

	_, err = env.SecondFactor.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code})

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	status, err := env.SecondFactor.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.True(t, status.Pending)
	assert.Zero(t, status.RecoveryCodesRemaining)
	issued, err := env.SecondFactor.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactors.ConfirmInput{EnrollmentID: second.ID, Code: code})
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch+1, issued.User.SessionEpoch)
	assert.NotEmpty(t, issued.RecoveryCodes)
}

func TestLifecycleAnAuditFailureRollsBackConfirmationAndCodes(t *testing.T) {
	r := newFactorRace(t)
	enrollment, err := r.factors.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
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

	_, err = r.factors.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactors.ConfirmInput{EnrollmentID: enrollment.ID, Code: code})

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
