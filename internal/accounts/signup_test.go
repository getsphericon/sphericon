package accounts_test

import (
	"database/sql"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// freshInstance is Accounts over a migrated database with no User, committing for real: the
// sign-up rule is about what an Instance holds, so the fixtures' Users would hide it.
func freshInstance(t *testing.T, rule accounts.SignupRule) *accounts.Accounts {
	t.Helper()
	sqlDB, err := sql.Open("pgx", testhelper.EmptyDBURL(t))
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })
	_, err = db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	return accounts.New(db.NewEntClient(sqlDB), events.New(sqlDB), accounts.WithSignupRule(rule))
}

func TestSelfHostedSignupIsOpenOnlyUntilTheFirstUser(t *testing.T) {
	acc := freshInstance(t, accounts.SignupUntilFirstUser)

	open, err := acc.SignupOpen(t.Context())
	require.NoError(t, err)
	require.True(t, open, "a fresh Instance lets its first Owner sign up")
	_, err = acc.SignUpVerified(t.Context(), "Ada", "ada@example.com", "hash", false)
	require.NoError(t, err)

	open, err = acc.SignupOpen(t.Context())
	require.NoError(t, err)
	assert.False(t, open, "the first User closes sign-up")
	_, err = acc.SignUpVerified(t.Context(), "Grace", "grace@example.com", "hash", false)
	require.ErrorIs(t, err, accounts.ErrSignupClosed)
	taken, err := acc.EmailTaken(t.Context(), "grace@example.com")
	require.NoError(t, err)
	assert.False(t, taken, "a refused sign-up creates no User")
}

func TestCreateOwnerStillWorksOnAnInstanceWhoseSignupIsClosed(t *testing.T) {
	acc := freshInstance(t, accounts.SignupUntilFirstUser)
	_, err := acc.SignUpVerified(t.Context(), "Ada", "ada@example.com", "hash", false)
	require.NoError(t, err)

	_, err = acc.CreateOwner(t.Context(), "grace@example.com", "Grace")

	assert.NoError(t, err, "create-owner is no public sign-up: the rule does not apply")
}

func TestTwoFirstSignupsOfAFreshInstanceMakeOneUser(t *testing.T) {
	acc := freshInstance(t, accounts.SignupUntilFirstUser)

	results := make([]error, 2)
	var wg sync.WaitGroup
	for i, email := range []string{"ada@example.com", "grace@example.com"} {
		wg.Go(func() {
			_, results[i] = acc.SignUpVerified(t.Context(), "Name", email, "hash", false)
		})
	}
	wg.Wait()

	assert.ElementsMatch(t, []error{nil, accounts.ErrSignupClosed}, results,
		"exactly one of two concurrent first sign-ups becomes the Owner, the other is refused")
}

func TestSaaSSignupStaysOpen(t *testing.T) {
	acc := freshInstance(t, accounts.SignupAlways)

	for _, email := range []string{"ada@example.com", "grace@example.com"} {
		_, err := acc.SignUpVerified(t.Context(), "Name", email, "hash", false)
		require.NoError(t, err)
	}

	open, err := acc.SignupOpen(t.Context())
	require.NoError(t, err)
	assert.True(t, open)
}
