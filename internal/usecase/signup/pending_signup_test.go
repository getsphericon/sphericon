package signup_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// openReference is the signed reference of the open fixture pending sign-up.
func (r rig) openReference(t *testing.T) string {
	t.Helper()
	ref, err := r.links.PendingSignupReference(r.db.PendingSignup.GetX(t.Context(), fixtures.PendingSignupOpenID))
	require.NoError(t, err)
	return ref
}

// ADR 0062: the answers complete the pending sign-up: they are checked as the provider screen's,
// and the User they make is signed in, recorded as `user.login` by the provider in its first
// Workspace, as at the end of a provider sign-in.
func TestCompletingAPendingSignupChecksTheAnswersAndRecordsTheLogin(t *testing.T) {
	r := newRig(t, 5)
	r.withItems(t)

	done, err := r.module.CompletePendingSignup(t.Context(), r.openReference(t), []consentrecords.Answer{termsAccepted, newsUnticked})

	require.NoError(t, err)
	assert.Equal(t, fixtures.PendingSignupOpenEmail, done.User.Email)
	assert.Equal(t, fixtures.PendingSignupOpenProvider, done.PendingSignup.Provider)
	assert.Equal(t, []string{
		fixtures.ServiceTermsKey + "@" + fixtures.ServiceTermsCurrentLabel + ": accepted from provider_screen",
		fixtures.OperatorNewsKey + "@" + fixtures.OperatorNewsCurrentLabel + ": declined from provider_screen",
		fixtures.DataNoticeKey + "@" + fixtures.DataNoticeCurrentLabel + ": shown from provider_screen",
	}, r.decided(t, fixtures.PendingSignupOpenEmail))
	r.env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	logins := r.env.DB.AuditEntry.Query().Where(auditentry.Action(events.ActionUserLogin), auditentry.TargetID(testhelper.ID(done.User.ID))).AllX(t.Context())
	require.Len(t, logins, 1, "one login, in the User's first Workspace")
	assert.Equal(t, fixtures.PendingSignupOpenProvider, logins[0].Diff["method"])
}

// A submit without the required acceptance is refused before anything is written: no User, no
// login, and the pending sign-up stays open for the corrected answers.
func TestAPendingSignupWithoutTheRequiredAcceptanceIsRefusedAndStaysOpen(t *testing.T) {
	r := newRig(t, 5)
	r.withItems(t)

	_, err := r.module.CompletePendingSignup(t.Context(), r.openReference(t), []consentrecords.Answer{newsUnticked})

	assert.Equal(t, []string{"signupItems." + fixtures.ServiceTermsKey}, refusedFields(t, err))
	assert.False(t, r.db.User.Query().Where(user.Email(fixtures.PendingSignupOpenEmail)).ExistX(t.Context()))
	_, err = r.module.CompletePendingSignup(t.Context(), r.openReference(t), []consentrecords.Answer{termsAccepted})
	require.NoError(t, err, "the corrected answers complete it")
}

// A reference that names nothing open completes nothing, before the answers are checked.
func TestAReferenceThatNamesNothingOpenCompletesNothing(t *testing.T) {
	r := newRig(t, 5)
	r.withItems(t)

	_, err := r.module.CompletePendingSignup(t.Context(), "forged", nil)

	require.ErrorIs(t, err, users.ErrPendingSignupGone)
}
