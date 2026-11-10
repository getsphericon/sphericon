package signup_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestCreateOwnerRefusesAMalformedEmail(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)

	_, err := env.SignUp.CreateOwner(t.Context(), "not-an-email", "Ada")

	require.ErrorIs(t, err, validation.ErrEmailInvalid)
	taken, err := acc.EmailTaken(t.Context(), "not-an-email")
	require.NoError(t, err)
	assert.False(t, taken, "a refusal creates no User")
}

func TestCreateOwnerMakesTheUserAndReturnsItsOneTimePassword(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)

	password, err := env.SignUp.CreateOwner(t.Context(), "ada@example.com", "Ada Lovelace")

	require.NoError(t, err)
	assert.NotEmpty(t, password)
	taken, err := acc.EmailTaken(t.Context(), "ada@example.com")
	require.NoError(t, err)
	assert.True(t, taken)
}

func TestCreateOwnerOfATakenEmailIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	password, err := env.SignUp.CreateOwner(t.Context(), fixtures.OwnerJohnEmail, "John")

	require.ErrorIs(t, err, users.ErrEmailTaken)
	assert.Empty(t, password)
}

func TestCreateOwnerRefusesANameEmptyToTheEye(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)

	password, err := env.SignUp.CreateOwner(t.Context(), "ada@example.com", "   ")

	require.ErrorIs(t, err, validation.ErrUserNameRequired)
	assert.Empty(t, password)
	taken, err := acc.EmailTaken(t.Context(), "ada@example.com")
	require.NoError(t, err)
	assert.False(t, taken, "a refusal creates no User")
}
