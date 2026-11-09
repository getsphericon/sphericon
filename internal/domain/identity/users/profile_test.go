package users_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupitems"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var (
	errWrongPassword = errors.New("the password is not the user's")
	errThrottled     = errors.New("the address's delay is running")
)

// proveByHash is the proof of the current password against the stored hash, as the login
// module's Prove checks it once its throttle lets it.
type proveByHash struct{}

func (proveByHash) ProvePassword(_ context.Context, _, passwordHash, password string) error {
	if credentials.VerifyPassword(passwordHash, password) {
		return nil
	}
	return errWrongPassword
}

// proveThrottled is the proof while the throttle's delay runs: refused whatever the password.
type proveThrottled struct{}

func (proveThrottled) ProvePassword(context.Context, string, string, string) error {
	return errThrottled
}

type profileRig struct {
	env  *testhelper.TestEnv
	acc  *users.Module
	john *ent.User
}

// newProfileRig is the Users over the test's database, whose current password is proven by prove.
func newProfileRig(t *testing.T, prove users.Prover) profileRig {
	t.Helper()
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now, users.WithProver(prove))
	john, err := acc.User(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	return profileRig{env: env, acc: acc, john: john}
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
	r := newProfileRig(t, proveByHash{})

	changed, err := r.acc.ChangeProfile(t.Context(), r.john, users.ProfileChange{Name: new("  Johnny  ")})

	require.NoError(t, err)
	assert.Equal(t, "Johnny", r.stored(t).Name)
	assert.False(t, changed.PasswordChanged, "a name change keeps every session")
}

func TestABlankProfileNameIsRefused(t *testing.T) {
	r := newProfileRig(t, proveByHash{})

	_, err := r.acc.ChangeProfile(t.Context(), r.john, users.ProfileChange{Name: new("   ")})

	assert.Equal(t, map[string][]string{"name": {i18n.T(i18n.ErrorsNameEmpty, nil)}}, refusedFields(t, err))
	assert.Equal(t, r.john.Name, r.stored(t).Name)
}

// The password rule is checked before the current password is proven: a refused one costs no
// proof, so it neither hashes nor counts against the Login budget.
func TestANewPasswordOutsideTheRuleIsRefusedBeforeAnyProof(t *testing.T) {
	r := newProfileRig(t, proveThrottled{})

	_, err := r.acc.ChangeProfile(t.Context(), r.john,
		users.ProfileChange{NewPassword: "short", CurrentPassword: fixtures.OwnerJohnPassword})

	refused := slices.Sorted(maps.Keys(refusedFields(t, err)))
	assert.Equal(t, []string{"newPassword"}, refused)
	assert.Equal(t, r.john.PasswordHash, r.stored(t).PasswordHash)
}

func TestANewPasswordNeedsTheCurrentOne(t *testing.T) {
	r := newProfileRig(t, proveByHash{})

	_, err := r.acc.ChangeProfile(t.Context(), r.john, users.ProfileChange{NewPassword: "newsecret123"})

	assert.Equal(t, map[string][]string{"currentPassword": {i18n.T(i18n.ErrorsCurrentPasswordRequired, nil)}}, refusedFields(t, err))
	assert.Equal(t, r.john.PasswordHash, r.stored(t).PasswordHash)
}

func TestAWrongCurrentPasswordRefusesTheChangeAndKeepsTheName(t *testing.T) {
	r := newProfileRig(t, proveByHash{})

	_, err := r.acc.ChangeProfile(t.Context(), r.john,
		users.ProfileChange{Name: new("Mallory"), NewPassword: "newsecret123", CurrentPassword: "wrong"})

	require.ErrorIs(t, err, errWrongPassword)
	stored := r.stored(t)
	assert.Equal(t, r.john.PasswordHash, stored.PasswordHash)
	assert.Equal(t, r.john.Name, stored.Name, "nothing is written unless every rule holds")
}

// While the Login throttle's delay runs the proof's own refusal comes back, even for the
// right password, and nothing changes.
func TestAThrottledProofRefusesEvenTheRightPassword(t *testing.T) {
	r := newProfileRig(t, proveThrottled{})

	_, err := r.acc.ChangeProfile(t.Context(), r.john,
		users.ProfileChange{NewPassword: "newsecret123", CurrentPassword: fixtures.OwnerJohnPassword})

	require.ErrorIs(t, err, errThrottled)
	assert.Equal(t, r.john.PasswordHash, r.stored(t).PasswordHash)
}

// ADR 0020: a new password ends every session, so the acting one must be issued again.
func TestAProvenPasswordChangeEndsEverySession(t *testing.T) {
	r := newProfileRig(t, proveByHash{})

	changed, err := r.acc.ChangeProfile(t.Context(), r.john,
		users.ProfileChange{NewPassword: "newsecret123", CurrentPassword: fixtures.OwnerJohnPassword})

	require.NoError(t, err)
	assert.True(t, changed.PasswordChanged)
	stored := r.stored(t)
	assert.True(t, credentials.VerifyPassword(stored.PasswordHash, "newsecret123"))
	assert.Equal(t, r.john.SessionEpoch+1, stored.SessionEpoch)
	assert.Equal(t, stored.SessionEpoch, changed.User.SessionEpoch, "the re-issued session is stamped with the new epoch")
}

func TestAProfileChangeAskingForNothingWritesNothing(t *testing.T) {
	r := newProfileRig(t, proveThrottled{})

	changed, err := r.acc.ChangeProfile(t.Context(), r.john, users.ProfileChange{CurrentPassword: "ignored"})

	require.NoError(t, err)
	assert.False(t, changed.PasswordChanged)
	assert.Equal(t, r.john.UpdatedAt, r.stored(t).UpdatedAt)
}

// A profile change answers with the required acceptances pending for the User (ADR 0062), which
// their profile shows.
func TestAProfileChangeAnswersWithTheAcceptancesPending(t *testing.T) {
	r := newProfileRig(t, proveByHash{})
	r.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	changed, err := r.acc.ChangeProfile(t.Context(), r.john, users.ProfileChange{Name: new("Johnny")})

	require.NoError(t, err)
	keys := lo.Map(changed.Pending, func(d signupitems.Document, _ int) string { return d.Key })
	assert.Equal(t, []string{fixtures.ServiceTermsKey}, keys)
}

func TestAUserWithAPasswordProvesTheirIdentityWithIt(t *testing.T) {
	r := newProfileRig(t, proveByHash{})

	right := r.acc.ProveIdentity(t.Context(), r.john, users.IdentityProof{Password: fixtures.OwnerJohnPassword})
	wrong := r.acc.ProveIdentity(t.Context(), r.john, users.IdentityProof{Password: "wrong", ProviderLoginAt: time.Now()})

	require.NoError(t, right)
	require.ErrorIs(t, wrong, errWrongPassword, "a fresh session proof never stands in for a password the User has")
}

// ADR 0048: a User with no password proves their identity with a fresh session proof, which
// nothing proves by a password, so the Login budget is not consulted.
func TestAUserWithNoPasswordProvesTheirIdentityWithAFreshSessionProof(t *testing.T) {
	r := newProfileRig(t, proveThrottled{})
	pia, err := r.acc.User(t.Context(), fixtures.ProviderOnlyPiaID)
	require.NoError(t, err)

	fresh := r.acc.ProveIdentity(t.Context(), pia, users.IdentityProof{PasskeyCheckAt: time.Now()})
	stale := r.acc.ProveIdentity(t.Context(), pia, users.IdentityProof{Password: "anything"})

	require.NoError(t, fresh)
	require.ErrorIs(t, stale, users.ErrIdentityProofRequired)
}
