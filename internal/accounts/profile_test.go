package accounts_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/validation"
)

var (
	errWrongPassword = errors.New("the password is not the user's")
	errThrottled     = errors.New("the address's delay is running")
)

// proveByHash is the proof of the current password against the stored hash, as the login
// module's Prove checks it once its throttle lets it.
func proveByHash(_ context.Context, u *ent.User, password string) error {
	if credentials.VerifyPassword(u.PasswordHash, password) {
		return nil
	}
	return errWrongPassword
}

// proveThrottled is the proof while the throttle's delay runs: refused whatever the password.
func proveThrottled(context.Context, *ent.User, string) error { return errThrottled }

type profileRig struct {
	acc  *accounts.Accounts
	john *ent.User
}

func newProfileRig(t *testing.T) profileRig {
	t.Helper()
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	john, err := acc.User(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	return profileRig{acc: acc, john: john}
}

func (r profileRig) stored(t *testing.T) *ent.User {
	t.Helper()
	u, err := r.acc.User(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	return u
}

// refusedFields is the field errors of a refused change.
func refusedFields(t *testing.T, err error) map[string][]string {
	t.Helper()
	fields, ok := errors.AsType[*validation.FieldsError](err)
	require.True(t, ok, "got %v", err)
	return fields.Fields
}

func TestAProfileNameIsStoredTrimmed(t *testing.T) {
	r := newProfileRig(t)

	changed, err := r.acc.ChangeProfile(t.Context(), r.john, accounts.ProfileChange{Name: new("  Johnny  ")}, proveByHash)

	require.NoError(t, err)
	assert.Equal(t, "Johnny", r.stored(t).Name)
	assert.False(t, changed.PasswordChanged, "a name change keeps every session")
}

func TestABlankProfileNameIsRefused(t *testing.T) {
	r := newProfileRig(t)

	_, err := r.acc.ChangeProfile(t.Context(), r.john, accounts.ProfileChange{Name: new("   ")}, proveByHash)

	assert.Equal(t, map[string][]string{"name": {i18n.T(i18n.ErrorsNameEmpty, nil)}}, refusedFields(t, err))
	assert.Equal(t, r.john.Name, r.stored(t).Name)
}

// The password rule is checked before the current password is proven: a refused one costs no
// proof, so it neither hashes nor counts against the Login budget.
func TestANewPasswordOutsideTheRuleIsRefusedBeforeAnyProof(t *testing.T) {
	r := newProfileRig(t)

	_, err := r.acc.ChangeProfile(t.Context(), r.john,
		accounts.ProfileChange{NewPassword: "short", CurrentPassword: fixtures.OwnerJohnPassword}, proveThrottled)

	refused := slices.Sorted(maps.Keys(refusedFields(t, err)))
	assert.Equal(t, []string{"newPassword"}, refused)
	assert.Equal(t, r.john.PasswordHash, r.stored(t).PasswordHash)
}

func TestANewPasswordNeedsTheCurrentOne(t *testing.T) {
	r := newProfileRig(t)

	_, err := r.acc.ChangeProfile(t.Context(), r.john, accounts.ProfileChange{NewPassword: "newsecret123"}, proveByHash)

	assert.Equal(t, map[string][]string{"currentPassword": {i18n.T(i18n.ErrorsCurrentPasswordRequired, nil)}}, refusedFields(t, err))
	assert.Equal(t, r.john.PasswordHash, r.stored(t).PasswordHash)
}

func TestAWrongCurrentPasswordRefusesTheChangeAndKeepsTheName(t *testing.T) {
	r := newProfileRig(t)

	_, err := r.acc.ChangeProfile(t.Context(), r.john,
		accounts.ProfileChange{Name: new("Mallory"), NewPassword: "newsecret123", CurrentPassword: "wrong"}, proveByHash)

	require.ErrorIs(t, err, errWrongPassword)
	stored := r.stored(t)
	assert.Equal(t, r.john.PasswordHash, stored.PasswordHash)
	assert.Equal(t, r.john.Name, stored.Name, "nothing is written unless every rule holds")
}

// While the Login throttle's delay runs the proof's own refusal comes back, even for the
// right password, and nothing changes.
func TestAThrottledProofRefusesEvenTheRightPassword(t *testing.T) {
	r := newProfileRig(t)

	_, err := r.acc.ChangeProfile(t.Context(), r.john,
		accounts.ProfileChange{NewPassword: "newsecret123", CurrentPassword: fixtures.OwnerJohnPassword}, proveThrottled)

	require.ErrorIs(t, err, errThrottled)
	assert.Equal(t, r.john.PasswordHash, r.stored(t).PasswordHash)
}

// ADR 0020: a new password ends every session, so the acting one must be issued again.
func TestAProvenPasswordChangeEndsEverySession(t *testing.T) {
	r := newProfileRig(t)

	changed, err := r.acc.ChangeProfile(t.Context(), r.john,
		accounts.ProfileChange{NewPassword: "newsecret123", CurrentPassword: fixtures.OwnerJohnPassword}, proveByHash)

	require.NoError(t, err)
	assert.True(t, changed.PasswordChanged)
	stored := r.stored(t)
	assert.True(t, credentials.VerifyPassword(stored.PasswordHash, "newsecret123"))
	assert.Equal(t, r.john.SessionEpoch+1, stored.SessionEpoch)
	assert.Equal(t, stored.SessionEpoch, changed.User.SessionEpoch, "the re-issued session is stamped with the new epoch")
}

func TestAProfileChangeAskingForNothingWritesNothing(t *testing.T) {
	r := newProfileRig(t)

	changed, err := r.acc.ChangeProfile(t.Context(), r.john, accounts.ProfileChange{CurrentPassword: "ignored"}, proveThrottled)

	require.NoError(t, err)
	assert.False(t, changed.PasswordChanged)
	assert.Equal(t, r.john.UpdatedAt, r.stored(t).UpdatedAt)
}
