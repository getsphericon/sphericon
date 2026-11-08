package secondfactorcheck_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/secondfactorcheck"
)

func TestLifecycleResetCancelsPendingEnrollment(t *testing.T) {
	env, _ := setup(t)
	_, err := env.SecondFactor.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	before, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)

	err = env.SecondFactor.ResetByStaff(t.Context(), fixtures.OwnerJohnID, "cli")

	require.NoError(t, err)
	status, err := env.SecondFactor.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.False(t, status.Pending)
	assert.False(t, status.Enabled)
	after, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch+1, after.SessionEpoch)
}

type factorRace struct {
	client  *ent.Client
	bus     *events.Bus
	usecase *secondfactorcheck.UseCase
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
		usecase: secondfactorcheck.New(client, bus, testhelper.Cipher(t), clock)}
}

// Pause the matching write after its operation loaded and checked the User. Register on both
// clients: the pre-fix Start writes directly, while transactional moves use the Bus client.
func (r factorRace) pause(t *testing.T, matches func(*ent.UserMutation) bool) (<-chan struct{}, func()) {
	t.Helper()
	ready, resume := make(chan struct{}, 1), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	t.Cleanup(release)
	var once sync.Once
	hook := func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if u, ok := mutation.(*ent.UserMutation); ok && matches(u) {
				once.Do(func() {
					ready <- struct{}{}
					select {
					case <-resume:
					case <-ctx.Done():
					}
				})
			}
			return next.Mutate(ctx, mutation)
		})
	}
	r.client.User.Use(hook)
	r.bus.Use(hook)
	return ready, release
}

func startsEnrollment(m *ent.UserMutation) bool {
	_, set := m.SecondFactorSecretEncrypted()
	return set
}

func acceptsStep(m *ent.UserMutation) bool {
	step, set := m.SecondFactorLastStep()
	return set && step > 0
}

func enrollmentCode(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, at)
	require.NoError(t, err)
	return code
}

func TestLifecycleAnOlderStartCannotReplaceAConfirmedFactor(t *testing.T) {
	r := newFactorRace(t)
	first, err := r.usecase.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	ready, release := r.pause(t, startsEnrollment)
	result := make(chan error, 1)
	go func() {
		_, err := r.usecase.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
		result <- err
	}()
	testhelper.Receive(t, ready, "replacement enrollment reached its conditional write")
	code := enrollmentCode(t, first.Secret, r.now)
	_, err = r.usecase.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactorcheck.ConfirmInput{EnrollmentID: first.ID, Code: code})
	require.NoError(t, err)
	release()

	stale := testhelper.Receive(t, result, "older Start finished")

	require.ErrorIs(t, stale, lifecycle.ErrInvalidTransition)
	status, err := r.usecase.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.True(t, status.Enabled)
	assert.False(t, status.Pending)
}

func TestLifecycleConfirmationCannotActivateAReplacementSecret(t *testing.T) {
	r := newFactorRace(t)
	first, err := r.usecase.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	ready, release := r.pause(t, acceptsStep)
	code := enrollmentCode(t, first.Secret, r.now)
	result := make(chan error, 1)
	go func() {
		_, err := r.usecase.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactorcheck.ConfirmInput{EnrollmentID: first.ID, Code: code})
		result <- err
	}()
	testhelper.Receive(t, ready, "confirmation checked the original secret")
	_, err = r.usecase.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	release()

	stale := testhelper.Receive(t, result, "old confirmation finished")

	require.ErrorIs(t, stale, lifecycle.ErrInvalidTransition)
	status, err := r.usecase.Status(t.Context(), fixtures.OwnerJohnID)
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

	_, err = env.SecondFactor.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactorcheck.ConfirmInput{EnrollmentID: first.ID, Code: code})

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	status, err := env.SecondFactor.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.True(t, status.Pending)
	assert.Zero(t, status.RecoveryCodesRemaining)
	issued, err := env.SecondFactor.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactorcheck.ConfirmInput{EnrollmentID: second.ID, Code: code})
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch+1, issued.User.SessionEpoch)
	assert.NotEmpty(t, issued.RecoveryCodes)
}

func TestLifecycleResetNeverReusesAnEnrollmentIdentity(t *testing.T) {
	env, c := setup(t)
	first, err := env.SecondFactor.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	err = env.SecondFactor.ResetByStaff(t.Context(), fixtures.OwnerJohnID, "cli")
	require.NoError(t, err)
	second, err := env.SecondFactor.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Greater(t, second.ID, first.ID)
	code := enrollmentCode(t, second.Secret, c.now())

	_, err = env.SecondFactor.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactorcheck.ConfirmInput{EnrollmentID: first.ID, Code: code})

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
}

func TestLifecycleAnAuditFailureRollsBackConfirmationAndCodes(t *testing.T) {
	r := newFactorRace(t)
	enrollment, err := r.usecase.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	before, err := r.client.User.Get(t.Context(), fixtures.OwnerJohnID)
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

	_, err = r.usecase.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactorcheck.ConfirmInput{EnrollmentID: enrollment.ID, Code: code})

	require.ErrorIs(t, err, events.ErrNoJobQueue)
	after, err := r.client.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Nil(t, after.SecondFactorConfirmedAt)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch)
	assert.Equal(t, before.SecondFactorLastStep, after.SecondFactorLastStep)
	codes, err := r.client.User.QueryRecoveryCodes(after).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, codes)
}

type factorAuditArgs struct{ events.EventJob }

func (factorAuditArgs) Kind() string { return "test_factor_audit" }

func TestLifecycleConfirmationCannotUndoAReset(t *testing.T) {
	r := newFactorRace(t)
	first, err := r.usecase.StartEnrollment(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	ready, release := r.pause(t, acceptsStep)
	code := enrollmentCode(t, first.Secret, r.now)
	result := make(chan error, 1)
	go func() {
		_, err := r.usecase.ConfirmEnrollment(t.Context(), fixtures.OwnerJohnID, secondfactorcheck.ConfirmInput{EnrollmentID: first.ID, Code: code})
		result <- err
	}()
	testhelper.Receive(t, ready, "confirmation checked the secret before reset")
	err = r.usecase.ResetByStaff(t.Context(), fixtures.OwnerJohnID, "cli")
	require.NoError(t, err)
	release()

	stale := testhelper.Receive(t, result, "confirmation after reset finished")

	require.ErrorIs(t, stale, lifecycle.ErrInvalidTransition)
	status, err := r.usecase.Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.False(t, status.Enabled)
	assert.False(t, status.Pending)
	assert.Zero(t, status.RecoveryCodesRemaining)
}

func regeneratesFactorCodes(m *ent.UserMutation) bool {
	_, step := m.SecondFactorLastStep()
	epoch, adds := m.AddedSessionEpoch()
	return adds && epoch > 0 && !step
}

func TestLifecycleRegenerationCannotRecreateCodesAfterReset(t *testing.T) {
	r := newFactorRace(t)
	ready, release := r.pause(t, regeneratesFactorCodes)
	result := make(chan error, 1)
	go func() {
		_, err := r.usecase.RegenerateRecoveryCodes(t.Context(), fixtures.SecondFactorSamID)
		result <- err
	}()
	testhelper.Receive(t, ready, "regeneration checked the active factor")
	err := r.usecase.ResetByStaff(t.Context(), fixtures.SecondFactorSamID, "cli")
	require.NoError(t, err)
	release()

	stale := testhelper.Receive(t, result, "regeneration after reset finished")

	require.ErrorIs(t, stale, lifecycle.ErrInvalidTransition)
	u, err := r.client.User.Get(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Empty(t, u.SecondFactorSecretEncrypted)
	codes, err := r.client.User.QueryRecoveryCodes(u).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, codes)
}

func TestLifecycleTOTPAndRecoveryCodeCannotBothSpendOneChallenge(t *testing.T) {
	r := newFactorRace(t)
	u, err := r.client.User.Get(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	challenged, err := r.usecase.ChallengeBinding(t.Context(), u)
	require.NoError(t, err)
	ready, release := r.pause(t, acceptsStep)
	totpResult, recoveryResult := make(chan error, 1), make(chan error, 1)
	code := enrollmentCode(t, fixtures.SecondFactorSamTotpSecret, r.now)
	go func() {
		_, err := r.usecase.Verify(t.Context(), pending(fixtures.SecondFactorSamID, challenged), code)
		totpResult <- err
	}()
	testhelper.Receive(t, ready, "TOTP verification holds the User lock")
	go func() {
		_, err := r.usecase.Verify(t.Context(), pending(fixtures.SecondFactorSamID, challenged), fixtures.SecondFactorSamRecoveryCode)
		recoveryResult <- err
	}()
	release()

	first := testhelper.Receive(t, totpResult, "TOTP verification finished")
	second := testhelper.Receive(t, recoveryResult, "recovery verification finished")

	require.NoError(t, first)
	require.ErrorIs(t, second, secondfactorcheck.ErrBindingMoved)
	status, err := r.usecase.Status(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Equal(t, 1, status.RecoveryCodesRemaining, "the losing challenge does not spend a recovery code")
}
