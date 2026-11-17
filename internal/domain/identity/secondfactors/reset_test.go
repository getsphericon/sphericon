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

// janeResets is Jane, the Owner of Globex alone, resetting a member's Second factor as actor.
func janeResets(actor events.Actor) secondfactors.Caller {
	return secondfactors.Caller{UserID: fixtures.OwnerJaneID, Role: membership.RoleOwner, Actor: actor}
}

// olga is Olga's actor: she owns Ollivanders and Gringotts, the two Workspaces of Reese.
var olga = events.Actor{Kind: events.ActorUser, ID: "olga", Name: "Olga"}

// olgaResets is Olga resetting a member's Second factor from Ollivanders.
func olgaResets() secondfactors.Caller {
	return secondfactors.Caller{UserID: fixtures.ResetOwnerOlgaID, Role: membership.RoleOwner, Actor: olga}
}

// resetReese is a reset of Reese's Second factor from Ollivanders by caller.
func resetReese(t *testing.T, env *testhelper.TestEnv, caller secondfactors.Caller) error {
	t.Helper()
	return testhelper.Module[*secondfactors.Module](env).Reset(t.Context(), env.DB.Scoped(fixtures.OllivandersID), fixtures.OllivandersReeseMembershipID, caller)
}

// resetEntries are the `user.second_factor_reset` Audit entries, oldest Workspace first.
func resetEntries(t *testing.T, env *testhelper.TestEnv) []*ent.AuditEntry {
	t.Helper()
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	got, err := env.DB.AuditEntry.Query().
		Where(auditentry.Action(events.ActionUserSecondFactorReset)).
		Order(ent.Asc(auditentry.FieldWorkspaceID)).
		All(t.Context())
	require.NoError(t, err)
	return got
}

// Nobody resets their own Second factor as a member's: they disable it in their account, which
// takes a password and a code. Nothing changes.
func TestResetOfTheCallersOwnSecondFactorIsRefused(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	sam := secondfactors.Caller{UserID: fixtures.SecondFactorSamID, Role: membership.RoleOwner, Actor: events.Actor{Kind: events.ActorUser, ID: "9"}}

	err := testhelper.Module[*secondfactors.Module](env).Reset(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.GlobexSamMembershipID, sam)

	require.ErrorIs(t, err, memberships.ErrResetOwnSecondFactor)
	st, err := testhelper.Module[*secondfactors.Module](env).Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.True(t, st.Enabled)
}

func TestResetClearsTheFactorAndEndsEverySession(t *testing.T) {
	env, c := setup(t)
	ctx := t.Context()
	session := env.SiteToken(t, fixtures.SecondFactorReeseEmail, nil)

	require.NoError(t, resetReese(t, env, olgaResets()))

	st, err := testhelper.Module[*secondfactors.Module](env).Status(ctx, fixtures.SecondFactorReeseID)
	require.NoError(t, err)
	assert.Equal(t, secondfactors.Status{}, st)
	code, err := totp.GenerateCode(fixtures.ReeseSecondFactorTotpSecret, c.Now())
	require.NoError(t, err)
	_, err = verify(t, env, fixtures.SecondFactorReeseID, code)
	require.ErrorIs(t, err, secondfactors.ErrNotActive)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/site/workspaces", nil)
	req.AddCookie(testhelper.RequestCookie("JWT", session))
	env.Server.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// The factor is the User's, so a reset is recorded in every Workspace of theirs, not only where
// it was made (ADR 0177).
func TestResetIsAuditedInEveryWorkspaceOfTheMember(t *testing.T) {
	env, _ := setup(t)

	require.NoError(t, resetReese(t, env, olgaResets()))

	got := resetEntries(t, env)
	require.Len(t, got, 2, "Reese belongs to Ollivanders and Gringotts")
	assert.Equal(t, int64(fixtures.OllivandersID), got[0].WorkspaceID)
	assert.Equal(t, int64(fixtures.GringottsID), got[1].WorkspaceID)
	for _, e := range got {
		assert.Equal(t, "Olga", *e.ActorName)
		assert.Equal(t, "user", e.TargetType)
		assert.Equal(t, "Reese", *e.TargetName)
	}
}

// The member learns of the reset from a mail, whoever made it and wherever (ADR 0177).
func TestResetMailsTheMember(t *testing.T) {
	env, _ := setup(t)

	require.NoError(t, resetReese(t, env, olgaResets()))

	sent := env.SystemMail.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, fixtures.SecondFactorReeseEmail, sent[0].To)
	assert.Contains(t, sent[0].Subject, "two-factor authentication of your Sphericon account was reset")
	assert.Contains(t, sent[0].Text, "every session of yours has ended")
}

// An Admin of both of the member's Workspaces may reset their Second factor.
func TestAnAdminOfEveryWorkspaceOfTheMemberResetsIt(t *testing.T) {
	env, _ := setup(t)
	abe := secondfactors.Caller{UserID: fixtures.ResetAdminAbeID, Role: membership.RoleAdmin, Actor: events.Actor{Kind: events.ActorUser, ID: "abe"}}

	require.NoError(t, resetReese(t, env, abe))

	st, err := testhelper.Module[*secondfactors.Module](env).Status(t.Context(), fixtures.SecondFactorReeseID)
	require.NoError(t, err)
	assert.False(t, st.Enabled)
}

// A caller who is no member of another Workspace of the member may not reset the factor that guards
// the member there: nothing changes, nothing is recorded and nobody is mailed (ADR 0177).
func TestResetIsRefusedWhenTheCallerIsNoMemberOfAnotherWorkspaceOfTheMember(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	before := env.DB.User.GetX(ctx, fixtures.SecondFactorSamID)

	err := testhelper.Module[*secondfactors.Module](env).Reset(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.GlobexSamMembershipID, janeResets(events.Actor{Kind: events.ActorUser, ID: "2"}))

	require.ErrorIs(t, err, memberships.ErrSecondFactorResetElsewhere, "Sam also belongs to Umbrella, where Jane is no member")
	st, err := testhelper.Module[*secondfactors.Module](env).Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.True(t, st.Enabled)
	after := env.DB.User.GetX(ctx, fixtures.SecondFactorSamID)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch, "no session ends")
	assert.Empty(t, resetEntries(t, env))
	assert.Empty(t, env.SystemMail.Messages())
}

// In another Workspace of the member the caller needs the same right as in this one: an Admin does
// not reset the factor of a member who is an Owner there (ADR 0177).
func TestResetIsRefusedWhenTheMemberIsAnOwnerOfAnotherWorkspaceTheCallerOnlyAdministers(t *testing.T) {
	env, _ := setup(t)
	env.Feed(t, func(tx *ent.Client) error {
		return tx.Membership.UpdateOneID(fixtures.GringottsReeseMembershipID).SetRole(membership.RoleOwner).Exec(t.Context())
	})
	abe := secondfactors.Caller{UserID: fixtures.ResetAdminAbeID, Role: membership.RoleAdmin, Actor: events.Actor{Kind: events.ActorUser, ID: "abe"}}

	err := resetReese(t, env, abe)

	require.ErrorIs(t, err, memberships.ErrSecondFactorResetElsewhere)
	st, err := testhelper.Module[*secondfactors.Module](env).Status(t.Context(), fixtures.SecondFactorReeseID)
	require.NoError(t, err)
	assert.True(t, st.Enabled)
}

func TestResetWithoutASecondFactorChangesNothing(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	before, err := env.DB.User.Get(ctx, fixtures.OwnerJohnID)
	require.NoError(t, err)

	err = testhelper.Module[*secondfactors.Module](env).Reset(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AcmeOwnerMembershipID, janeResets(events.Actor{Kind: events.ActorUser, ID: "2"}))
	require.ErrorIs(t, err, secondfactors.ErrNothingToReset)

	after, err := env.DB.User.Get(ctx, fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch, "no session ends")
}

func TestStaffResetIsAuditedInEveryWorkspaceOfTheUser(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()

	before := env.DB.User.GetX(ctx, fixtures.SecondFactorSamID)

	require.NoError(t, testhelper.Module[*secondfactors.Module](env).ResetByStaff(ctx, "Sam@Globex.test", "cli"), "the email is matched whatever its case")

	st, err := testhelper.Module[*secondfactors.Module](env).Status(ctx, fixtures.SecondFactorSamID)
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
	sent := env.SystemMail.Messages()
	require.Len(t, sent, 1, "Sam is told of the reset")
	assert.Equal(t, fixtures.SecondFactorSamEmail, sent[0].To)
}

func TestStaffResetOfAUserWithNothingToClearChangesNothing(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	require.NoError(t, testhelper.Module[*secondfactors.Module](env).ResetByStaff(ctx, fixtures.SecondFactorSamEmail, "cli"))

	err := testhelper.Module[*secondfactors.Module](env).ResetByStaff(ctx, fixtures.SecondFactorSamEmail, "cli")

	require.ErrorIs(t, err, secondfactors.ErrNothingToReset)
}

func TestStaffResetOfAnUnknownEmailNamesIt(t *testing.T) {
	env, _ := setup(t)

	err := testhelper.Module[*secondfactors.Module](env).ResetByStaff(t.Context(), "nobody@app-2fa-reset.test", "cli")

	require.ErrorContains(t, err, `user "nobody@app-2fa-reset.test"`)
	require.True(t, ent.IsNotFound(err))
}

func TestLifecycleResetCancelsPendingEnrollment(t *testing.T) {
	env, _ := setup(t)
	_, err := testhelper.Module[*secondfactors.Module](env).StartEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	before, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)

	err = testhelper.Module[*secondfactors.Module](env).ResetByStaff(t.Context(), fixtures.OwnerJohnEmail, "cli")

	require.NoError(t, err)
	status, err := testhelper.Module[*secondfactors.Module](env).Status(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.False(t, status.Pending)
	assert.False(t, status.Enabled)
	after, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch+1, after.SessionEpoch)
}

func TestLifecycleResetNeverReusesAnEnrollmentIdentity(t *testing.T) {
	env, c := setup(t)
	first, err := testhelper.Module[*secondfactors.Module](env).StartEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	err = testhelper.Module[*secondfactors.Module](env).ResetByStaff(t.Context(), fixtures.OwnerJohnEmail, "cli")
	require.NoError(t, err)
	second, err := testhelper.Module[*secondfactors.Module](env).StartEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	assert.Greater(t, second.ID, first.ID)
	code, err := totp.GenerateCode(second.Secret, c.Now())
	require.NoError(t, err)

	_, err = testhelper.Module[*secondfactors.Module](env).ConfirmEnrollment(t.Context(), env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID), secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code, Proof: users.IdentityProof{Password: fixtures.OwnerJohnPassword}})

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
		_, err := r.factors.ConfirmEnrollment(t.Context(), r.client.User.GetX(t.Context(), fixtures.OwnerJohnID), secondfactors.ConfirmInput{EnrollmentID: first.ID, Code: code, Proof: users.IdentityProof{Password: fixtures.OwnerJohnPassword}})
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

// ClearFactorAndPasskeysIn drops a User's factor, Recovery codes and Passkeys whatever they hold, and a User who
// holds none of them is no error (ADR 0170).
func TestClearFactorAndPasskeysInDropsWhateverTheUserHolds(t *testing.T) {
	for name, userID := range map[string]int64{
		"a factor and its codes": fixtures.SecondFactorSamID,
		"passkeys alone":         fixtures.PasskeyPatID,
		"nothing":                fixtures.OwnerJohnID,
	} {
		t.Run(name, func(t *testing.T) {
			env, _ := setup(t)
			ctx := t.Context()

			_, err := testhelper.Module[*secondfactors.Module](env).ClearFactorAndPasskeysIn(ctx, env.DB, userID)

			require.NoError(t, err)
			st, err := testhelper.Module[*secondfactors.Module](env).Status(ctx, userID)
			require.NoError(t, err)
			assert.Equal(t, secondfactors.Status{}, st)
			codes, err := env.DB.RecoveryCode.Query().Where(recoverycode.UserID(userID)).Count(ctx)
			require.NoError(t, err)
			assert.Zero(t, codes)
			held, err := env.DB.Passkey.Query().Where(passkey.UserID(userID)).Exist(ctx)
			require.NoError(t, err)
			assert.False(t, held)
		})
	}
}
