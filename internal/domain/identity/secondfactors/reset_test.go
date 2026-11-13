package secondfactors_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/passkey"
	"github.com/getsphericon/sphericon/ent/recoverycode"
	"github.com/getsphericon/sphericon/ent/secondfactor"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// janeResets is Jane, an Owner, resetting a member's Second factor as actor.
func janeResets(actor events.Actor) secondfactors.Caller {
	return secondfactors.Caller{UserID: fixtures.OwnerJaneID, Role: membership.RoleOwner, Actor: actor}
}

// Nobody resets their own Second factor as a member's: they disable it in their account, which
// takes a password and a code. Nothing changes.
func TestResetOfTheCallersOwnSecondFactorIsRefused(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	sam := secondfactors.Caller{UserID: fixtures.SecondFactorSamID, Role: membership.RoleOwner, Actor: events.Actor{Kind: events.ActorUser, ID: "9"}}

	err := env.SecondFactor.Reset(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.GlobexSamMembershipID, sam)

	require.ErrorIs(t, err, memberships.ErrResetOwnSecondFactor)
	st, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.True(t, st.Enabled)
}

func TestResetClearsTheFactorAndEndsEverySession(t *testing.T) {
	env, c := setup(t)
	ctx := t.Context()
	session := env.SiteToken(t, fixtures.SecondFactorSamEmail, nil)

	jane := events.Actor{Kind: events.ActorUser, ID: "2", Name: "Jane"}
	require.NoError(t, env.SecondFactor.Reset(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.GlobexSamMembershipID, janeResets(jane)))

	st, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Equal(t, secondfactors.Status{}, st)
	_, err = verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.ErrorIs(t, err, secondfactors.ErrNotActive)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/site/workspaces", nil)
	req.AddCookie(testhelper.RequestCookie("JWT", session))
	env.Server.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestResetIsAuditedInTheActingWorkspaceOnly(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	jane := events.Actor{Kind: events.ActorUser, ID: "2", Name: "Jane"}

	require.NoError(t, env.SecondFactor.Reset(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.GlobexSamMembershipID, janeResets(jane)))

	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	got, err := env.DB.AuditEntry.Query().Where(auditentry.Action(events.ActionUserSecondFactorReset)).All(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int64(fixtures.GlobexID), got[0].WorkspaceID)
	assert.Equal(t, "Jane", *got[0].ActorName)
	assert.Equal(t, "user", got[0].TargetType)
	assert.Equal(t, "Sam", *got[0].TargetName)
}

func TestResetWithoutASecondFactorChangesNothing(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	before, err := env.DB.User.Get(ctx, fixtures.OwnerJohnID)
	require.NoError(t, err)

	err = env.SecondFactor.Reset(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AcmeOwnerMembershipID, janeResets(events.Actor{Kind: events.ActorUser, ID: "2"}))
	require.ErrorIs(t, err, secondfactors.ErrNothingToReset)

	after, err := env.DB.User.Get(ctx, fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch, "no session ends")
}

func TestStaffResetIsAuditedInEveryWorkspaceOfTheUser(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()

	before := env.DB.User.GetX(ctx, fixtures.SecondFactorSamID)

	require.NoError(t, env.SecondFactor.ResetByStaff(ctx, "Sam@Globex.test", "cli"), "the email is matched whatever its case")

	st, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.False(t, st.Enabled)
	factor := env.DB.SecondFactor.GetX(ctx, fixtures.SamSecondFactorID)
	assert.Nil(t, factor.ConfirmedAt)
	assert.Empty(t, factor.SecretEncrypted)
	assert.Greater(t, env.DB.User.GetX(ctx, fixtures.SecondFactorSamID).SessionEpoch, before.SessionEpoch, "every session ends")
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	got, err := env.DB.AuditEntry.Query().
		Where(auditentry.Action(events.ActionUserSecondFactorReset)).
		Order(ent.Asc(auditentry.FieldWorkspaceID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2, "Sam belongs to Globex and Umbrella")
	assert.Equal(t, int64(fixtures.GlobexID), got[0].WorkspaceID)
	assert.Equal(t, int64(fixtures.UmbrellaID), got[1].WorkspaceID)
	for _, e := range got {
		assert.Equal(t, auditentry.ActorKindStaff, e.ActorKind)
	}
}

func TestStaffResetOfAUserWithNothingToClearChangesNothing(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	require.NoError(t, env.SecondFactor.ResetByStaff(ctx, fixtures.SecondFactorSamEmail, "cli"))

	err := env.SecondFactor.ResetByStaff(ctx, fixtures.SecondFactorSamEmail, "cli")

	require.ErrorIs(t, err, secondfactors.ErrNothingToReset)
}

func TestStaffResetOfAnUnknownEmailNamesIt(t *testing.T) {
	env, _ := setup(t)

	err := env.SecondFactor.ResetByStaff(t.Context(), "nobody@app-2fa-reset.test", "cli")

	require.ErrorContains(t, err, `user "nobody@app-2fa-reset.test"`)
	require.True(t, ent.IsNotFound(err))
}

func TestLifecycleResetCancelsPendingEnrollment(t *testing.T) {
	env, _ := setup(t)
	_, err := env.SecondFactor.StartEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	before, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)

	err = env.SecondFactor.ResetByStaff(t.Context(), fixtures.OwnerJohnEmail, "cli")

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
	err = env.SecondFactor.ResetByStaff(t.Context(), fixtures.OwnerJohnEmail, "cli")
	require.NoError(t, err)
	second, err := env.SecondFactor.StartEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	assert.Greater(t, second.ID, first.ID)
	code, err := totp.GenerateCode(second.Secret, c.now())
	require.NoError(t, err)

	_, err = env.SecondFactor.ConfirmEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword}, secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code})

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
}

func TestLifecycleConfirmationCannotUndoAReset(t *testing.T) {
	r := newFactorRace(t)
	first, err := r.factors.StartEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	ready, release := r.pause(t, endsSessions)
	code, err := totp.GenerateCode(first.Secret, r.now)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		_, err := r.factors.ConfirmEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword}, secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code})
		result <- err
	}()
	testhelper.Receive(t, ready, "confirmation checked the secret before reset")
	err = r.factors.ResetByStaff(t.Context(), fixtures.OwnerJohnEmail, "cli")
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
	ready, release := r.pause(t, endsSessions)
	result := make(chan error, 1)
	go func() {
		_, err := r.factors.RegenerateRecoveryCodes(t.Context(), r.client.User.GetX(t.Context(), fixtures.SecondFactorSamID), users.IdentityProof{Password: fixtures.SecondFactorSamPassword})
		result <- err
	}()
	testhelper.Receive(t, ready, "regeneration checked the active factor")
	err := r.factors.ResetByStaff(t.Context(), fixtures.SecondFactorSamEmail, "cli")
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

// A reset of a User who holds only Passkeys has no factor row to write, so its conditional write is
// the bump of the User's session epoch alone: a reset that checked the User before another reset
// moved it fails rather than ending their sessions a second time.
func TestLifecycleAStaleResetOfPasskeysAloneCannotWriteAgain(t *testing.T) {
	r := newFactorRace(t)
	ready, release := r.pause(t, endsSessions)
	result := make(chan error, 1)
	go func() {
		result <- r.factors.ResetByStaff(t.Context(), fixtures.PasskeyPatEmail, "cli")
	}()
	testhelper.Receive(t, ready, "the first reset checked Pat's Passkeys")
	before := r.client.User.GetX(t.Context(), fixtures.PasskeyPatID)
	require.NoError(t, r.factors.ResetByStaff(t.Context(), fixtures.PasskeyPatEmail, "cli"))
	release()

	stale := testhelper.Receive(t, result, "the first reset finished")

	require.ErrorIs(t, stale, lifecycle.ErrInvalidTransition)
	assert.Equal(t, before.SessionEpoch+1, r.client.User.GetX(t.Context(), fixtures.PasskeyPatID).SessionEpoch, "the sessions end once")
	held, err := r.client.Passkey.Query().Where(passkey.UserID(fixtures.PasskeyPatID)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, held)
}
